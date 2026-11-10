package e2e

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	testkit "github.com/go-while/nntp-testkit"
	"github.com/go-while/nntp-testkit/fakeserver"
)

// TestDeepRevSighupRequestInFlight (L4-07): on SIGHUP ("drcmd exit") a reader process drops
// its clients. It freed a client that still had an article request out at a spool server
// without taking the client off the request, and it goes through all its connections in the
// order of their descriptor numbers. When the server's answer was already there and the
// server link had the higher number (a link that was set up again after the client came),
// the answer was then written into the freed client: a fault in the reader, which in a
// shutdown nobody sees, except through "readercrash".
//
// The spool server is started after the client is connected, so its link has the higher
// descriptor. The reader is stopped (SIGSTOP) while the request is out, gets SIGHUP, and is
// continued once the server has answered, so it finds the signal and the answer together.
// The crash command must not run.
func TestDeepRevSighupRequestInFlight(t *testing.T) {
	requireProcessInspection(t, false)
	port := testkit.FreePort(t)
	addr := "127.0.0.1:" + strconv.Itoa(port)
	s := NewReaderSite(t, ReaderOptions{SpoolAddr: addr, NoStart: true,
		Config: map[string]string{"readercrash": "@home@/crash.sh"}})
	crashLog := filepath.Join(s.Dir, "crash.log")
	s.WriteFile(t, "crash.sh", "#!/bin/sh\necho \"$1\" >> "+crashLog+"\n")
	if err := os.Chmod(filepath.Join(s.Dir, "crash.sh"), 0o755); err != nil {
		t.Fatal(err)
	}
	_, sitePort, _ := strings.Cut(s.Addr, ":")
	startReader(t, s, "[127.0.0.1]:"+sitePort)
	deepRevKillChildren(t, s, "dreaderd")

	st := fakeserver.NewStore()
	st.AddGroup("e2e.test", "e2e test group")
	id := msgID(t)
	feedReader(t, s, st, "e2e.test", newArticle(id, "e2e.test", "sighup", "the body"))

	// The client first: it gets a lower descriptor in the reader than the spool link.
	c := s.Reader(t)
	testkit.WaitFor(t, testkit.Deadline(), "the article in GROUP e2e.test", func() bool {
		_, line := c.Cmd("GROUP e2e.test")
		f := strings.Fields(line)
		return len(f) > 1 && f[0] == "211" && f[1] != "0"
	})

	const answerAfter = 1500 * time.Millisecond
	spool := fakeserver.New(t, fakeserver.Options{Addr: addr, Store: st,
		Faults: fakeserver.Faults{Delay: map[string]time.Duration{"ARTICLE": answerAfter, "BODY": answerAfter}}})
	// Commands keep the reader's loop turning until it has tried the spool server again.
	limit := time.Now().Add(90 * time.Second)
	for spool.Open() == 0 {
		if time.Now().After(limit) {
			t.Fatalf("the reader did not connect to the spool server in 90 s\n%s", s.logOutput())
		}
		c.ExpectCmd(111, "DATE")
		time.Sleep(200 * time.Millisecond)
	}
	time.Sleep(2 * time.Second) // the link is set up (MODE READER and so on)

	var reader int
	for p := range readerForkPids(s) {
		reader, _ = strconv.Atoi(p)
	}
	if reader == 0 {
		t.Fatalf("no reader process in the output of dreaderd:\n%s", s.logOutput())
	}

	requests := spool.Count("ARTICLE") + spool.Count("BODY")
	if err := c.Send("BODY " + id + "\r\n"); err != nil {
		t.Fatal(err)
	}
	limit = time.Now().Add(10 * time.Second)
	for spool.Count("ARTICLE")+spool.Count("BODY") == requests {
		if time.Now().After(limit) {
			t.Fatalf("the reader sent no request for %s to the spool server in 10 s; it got: %q", id, spool.Lines())
		}
		time.Sleep(10 * time.Millisecond)
	}
	// The request is out, the answer comes in 1.5 s.
	if err := syscall.Kill(reader, syscall.SIGSTOP); err != nil {
		t.Fatalf("SIGSTOP to the reader %d: %v", reader, err)
	}
	_ = syscall.Kill(reader, syscall.SIGHUP)
	time.Sleep(answerAfter + time.Second)
	_ = syscall.Kill(reader, syscall.SIGCONT)

	// The reader ends (it has no client left); a fault would run the crash command.
	limit = time.Now().Add(10 * time.Second)
	for time.Now().Before(limit) {
		if b, _ := os.ReadFile(crashLog); len(b) > 0 {
			t.Fatalf("the reader process faulted after SIGHUP with a request in flight (the crash command ran): the answer of the spool server was written into the client connection it had just freed\n%s",
				generalLog(s))
		}
		if syscall.Kill(reader, 0) != nil {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	time.Sleep(500 * time.Millisecond)
	if b, _ := os.ReadFile(crashLog); len(b) > 0 {
		t.Fatalf("the reader process faulted after SIGHUP with a request in flight (the crash command ran)\n%s", generalLog(s))
	}
}

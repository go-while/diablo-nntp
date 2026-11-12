package e2e

import (
	"os"
	"path/filepath"
	"runtime"
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
// The client must be closed without the answer (the state above: freed while the answer was
// there), the reader must end, and the crash command must not run.
//
// A write into the freed connection did not always fault: the pool pages of the client are
// given back with munmap(), and the next mmap() (a buffer for the server's answer) often got
// the same address back, so the write went into live memory and the test passed (1 of 9 runs
// against the old code). Where an LD_PRELOAD injector can be used, munmap() makes the pages
// inaccessible instead (unmapTrapSource), so such a write faults every time.
func TestDeepRevSighupRequestInFlight(t *testing.T) {
	requireProcessInspection(t, false)
	port := testkit.FreePort(t)
	addr := "127.0.0.1:" + strconv.Itoa(port)
	s := NewReaderSite(t, ReaderOptions{SpoolAddr: addr, NoStart: true,
		Config: map[string]string{"readercrash": "@home@/crash.sh"}})
	mark := ""
	if preloadHost(true) {
		so := preloadShim(t, "unmaptrap", withMark(unmapTrapSource))
		mark = filepath.Join(t.TempDir(), "mark")
		s.Env = append(s.Env, lockFaultEnv(so), "SHIM_MARK="+mark)
	} else {
		t.Logf("no LD_PRELOAD on %s: a write into the freed client may go unnoticed", runtime.GOOS)
	}
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
	unmapped := readerUnmaps(mark, reader)
	_ = syscall.Kill(reader, syscall.SIGCONT)

	// The client is dropped before the answer reaches it.
	lines, err := c.ExpectClosed(10 * time.Second)
	for _, l := range lines {
		if strings.HasPrefix(l, "222") {
			t.Fatalf("the client got the answer (%q) before the reader dropped it: the test did not reach the state it is about (the answer there while the client is freed)", lines)
		}
	}
	if err != nil {
		t.Fatalf("the client was not closed after SIGHUP to its reader: %v", err)
	}

	// The reader ends (it has no client left); a fault would run the crash command.
	gone := false
	limit = time.Now().Add(10 * time.Second)
	for time.Now().Before(limit) {
		if b, _ := os.ReadFile(crashLog); len(b) > 0 {
			t.Fatalf("the reader process faulted after SIGHUP with a request in flight (the crash command ran): the answer of the spool server was written into the client connection it had just freed\n%s",
				generalLog(s))
		}
		if syscall.Kill(reader, 0) != nil {
			gone = true
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	time.Sleep(500 * time.Millisecond)
	if b, _ := os.ReadFile(crashLog); len(b) > 0 {
		t.Fatalf("the reader process faulted after SIGHUP with a request in flight (the crash command ran)\n%s", generalLog(s))
	}
	if !gone {
		t.Errorf("the reader %d still runs 10 s after SIGHUP: it did not drop its client", reader)
	}
	if mark != "" && readerUnmaps(mark, reader) <= unmapped {
		t.Errorf("the reader %d gave no memory back (munmap()) after SIGCONT: the client was not freed, or the injector did not act", reader)
	}
}

// unmapTrapSource makes munmap() leave the pages mapped but inaccessible (PROT_NONE), so a
// later use of freed pool memory faults instead of reaching whatever the next mmap() put
// there; each call is marked. The address space is not given back: for short tests only.
const unmapTrapSource = `#define _GNU_SOURCE
#define __EXTENSIONS__
#include <fcntl.h>
#include <stdarg.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/types.h>
#include <sys/mman.h>
#include <unistd.h>
/*MARK*/
int
munmap(void *addr, size_t len)
{
    shim_mark("munmap %lu", (unsigned long)len);
    return mprotect(addr, len, PROT_NONE);
}
`

// readerUnmaps counts the munmap() calls the injector marked for process pid (0 without a
// mark file).
func readerUnmaps(mark string, pid int) int {
	if mark == "" {
		return 0
	}
	n := 0
	for _, l := range shimMarked(mark, " munmap ") {
		if strings.HasPrefix(l, strconv.Itoa(pid)+" ") {
			n++
		}
	}
	return n
}

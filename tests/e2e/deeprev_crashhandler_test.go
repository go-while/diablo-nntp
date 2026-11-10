package e2e

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	testkit "github.com/go-while/nntp-testkit"
)

// TestDeepRevCrashHandlerForkLoop (L4-08): with "readercrash <command>" a reader process
// that gets SIGSEGV, SIGBUS, SIGFPE or SIGILL forks; the child runs the command with its pid
// and stays for inspection. The parent returned from the signal handler with the handler
// still installed, so it ran the faulting instruction again, came back into the handler and
// forked again, without end: one more process and one more run of the command per lap.
//
// A real fault is needed (a signal sent with kill does not come back after the handler
// returns): the reader maps the overview index of a group, the test cuts that file to zero
// bytes and asks for the overview again, which is SIGBUS. The crash command writes one line
// per run; exactly one line is right.
func TestDeepRevCrashHandlerForkLoop(t *testing.T) {
	requireProcessInspection(t, false)
	st, spool := spoolStore(t)
	s := NewReaderSite(t, ReaderOptions{Spool: spool, NoStart: true,
		Config: map[string]string{"readercrash": "@home@/crash.sh"}})
	crashLog := filepath.Join(s.Dir, "crash.log")
	s.WriteFile(t, "crash.sh", "#!/bin/sh\necho \"$1\" >> "+crashLog+"\n")
	if err := os.Chmod(filepath.Join(s.Dir, "crash.sh"), 0o755); err != nil {
		t.Fatal(err)
	}
	_, port, _ := strings.Cut(s.Addr, ":")
	startReader(t, s, "[127.0.0.1]:"+port)
	deepRevKillChildren(t, s, "dreaderd")

	runs := func() int {
		b, _ := os.ReadFile(crashLog)
		return strings.Count(string(b), "\n")
	}
	// Stop a fork loop at once: it does not end by itself.
	killChildren := func() {
		for range 20 {
			pids, _ := trySitePids(s, "dreaderd")
			for _, pid := range pids {
				if pid != s.Proc.Pid() {
					_ = syscall.Kill(pid, syscall.SIGKILL)
				}
			}
			time.Sleep(50 * time.Millisecond)
		}
	}

	feedReader(t, s, st, "e2e.test", newArticle(msgID(t), "e2e.test", "crash handler", "body"))
	c := s.Reader(t)
	testkit.WaitFor(t, testkit.Deadline(), "the article in GROUP e2e.test", func() bool {
		_, line := c.Cmd("GROUP e2e.test")
		f := strings.Fields(line)
		return len(f) > 1 && f[0] == "211" && f[1] != "0"
	})
	c.ExpectCmd(224, "OVER 1")
	c.ReadDotLines()

	// Cut the mapped overview index: the next access to the mapping is SIGBUS.
	cut := 0
	_ = filepath.WalkDir(filepath.Join(s.Dir, "spool", "group"), func(path string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() && strings.HasPrefix(d.Name(), "over.") {
			if os.Truncate(path, 0) == nil {
				cut++
			}
		}
		return nil
	})
	if cut == 0 {
		t.Fatalf("no overview index file (over.*) under %s/spool/group", s.Dir)
	}
	c.Deadline = 5 * time.Second
	_, _, _ = c.TryCmd("OVER 1")

	limit := time.Now().Add(15 * time.Second)
	for runs() == 0 {
		if time.Now().After(limit) {
			t.Fatalf("the crash command did not run in 15 s: the reader did not fault on the cut overview index, the test has lost its trigger\n%s",
				s.logOutput())
		}
		time.Sleep(10 * time.Millisecond)
	}
	// One run is right. Watch for more for two seconds.
	limit = time.Now().Add(2 * time.Second)
	for time.Now().Before(limit) {
		if n := runs(); n > 1 {
			killChildren()
			t.Fatalf("the crash command ran %d times (and counting) for one fault: the faulting reader returns from the signal handler into the fault and forks again without end",
				n)
		}
		time.Sleep(10 * time.Millisecond)
	}

	// The main process starts a new reader: a client is served again.
	r := s.Reader(t)
	r.ExpectCmd(111, "DATE")
	r.ExpectCmd(205, "QUIT")
}

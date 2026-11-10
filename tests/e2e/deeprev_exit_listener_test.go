package e2e

import (
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/go-while/nntp-testkit/fakeserver"
)

// TestDeepRevExitListenerSpin (L4-10): "drcmd exit" closed the listening sockets but left
// them in the main loop's select() set and descriptor table. From then on select() failed at
// once (EBADF) with the sets as they were, so every descriptor looked ready and the 50 ms
// tick of the shutdown was gone: the main process ran its loop without a pause until the
// last reader had ended, and a closed listener's number, once reused, was taken for a
// listener.
//
// A client in the middle of a POST keeps its reader process for about two seconds after the
// exit command. The main process must not use more than half a second of CPU time until it
// is gone.
func TestDeepRevExitListenerSpin(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("needs /proc/<pid>/stat for the CPU time of the main process")
	}
	_, spool := spoolStore(t)
	post := fakeserver.New(t, fakeserver.Options{Handler: fakeserver.PeerHandler(&fakeserver.PeerOptions{})})
	s := NewReaderSite(t, ReaderOptions{Spool: spool, Post: post})
	deepRevKillChildren(t, s, "dreaderd")
	pid := s.Proc.Pid()

	id := msgID(t)
	p := s.Reader(t)
	p.ExpectCmd(340, "POST")
	head := newArticle(id, "e2e.test", "exit while an article comes in")
	if err := p.Send(strings.Join(head[:3], "\r\n") + "\r\n"); err != nil {
		t.Fatal(err)
	}
	time.Sleep(500 * time.Millisecond) // the reader has the first lines

	before := cpuTicks(pid)
	if before < 0 {
		t.Fatalf("no CPU time for the main process %d", pid)
	}
	start := time.Now()
	// drcmd exit returns when dreaderd has ended: sample the CPU time meanwhile.
	exitErr := make(chan string, 1)
	go func() {
		if out, err := s.TryRun("drcmd", "exit"); err != nil {
			exitErr <- err.Error() + "\n" + out
		}
		close(exitErr)
	}()
	last := before
	for done := false; !done; {
		select {
		case <-s.Proc.Done():
			done = true
		case <-time.After(10 * time.Millisecond):
			if n := cpuTicks(pid); n > last {
				last = n
			}
			if time.Since(start) > 30*time.Second {
				t.Fatalf("dreaderd still runs 30 s after drcmd exit")
			}
		}
	}
	took := time.Since(start)
	if msg := <-exitErr; msg != "" {
		t.Fatalf("drcmd exit: %s", msg)
	}
	t.Logf("the main process ended %v after drcmd exit and used %d clock ticks of CPU time", took.Round(10*time.Millisecond), last-before)
	if took < time.Second {
		t.Fatalf("the main process ended %v after drcmd exit: the client in POST did not hold its reader, the test has lost its trigger", took)
	}
	if last-before > 50 {
		t.Errorf("the main process used %d clock ticks of CPU time in the %v between drcmd exit and its end: its loop runs without a pause on the closed listening socket",
			last-before, took.Round(10*time.Millisecond))
	}
}

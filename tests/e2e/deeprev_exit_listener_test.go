package e2e

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
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

// TestDeepRevExitPassedListener: "drcmd exit" closes the listeners (DelThread()) from inside
// DoCommand(), in the middle of the main loop's walk over the select() result. A listener
// whose descriptor is higher than the drcmd socket's and that was readable in the same pass
// was then reached with no thread left, and the walk ended the main process in
// fatal("select returned unknown descriptor"), exit status 1, without the orderly shutdown.
// dreaderd -P opens its listeners before the drcmd socket (lower numbers); dreaderd -b <fd>
// serves one it inherited, whose number the parent chose.
//
// dreaderd -b 7 and -b 8 (between the descriptors dreaderd opens itself), three times each,
// with clients connecting in a loop so that the listener is readable when the exit command
// comes: the main process must end without that fatal().
func TestDeepRevExitPassedListener(t *testing.T) {
	for _, fd := range []int{7, 8} {
		for run := 1; run <= 3; run++ {
			t.Run(fmt.Sprintf("fd%d-%d", fd, run), func(t *testing.T) { exitPassedListener(t, fd) })
		}
	}
}

func exitPassedListener(t *testing.T, fd int) {
	_, spool := spoolStore(t)
	s := NewReaderSite(t, ReaderOptions{Spool: spool, NoStart: true})
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	f, err := l.(*net.TCPListener).File()
	if err != nil {
		t.Fatal(err)
	}
	addr := l.Addr().String()
	cmd := exec.Command(filepath.Join(s.Bin, "dreaderd"), "-d", "-p", PathHost, "-h", PathHost, "-b", strconv.Itoa(fd))
	cmd.ExtraFiles = make([]*os.File, fd-2) // descriptor 3+i is ExtraFiles[i]; the others closed
	cmd.ExtraFiles[fd-3] = f
	out, err := os.Create(filepath.Join(s.Dir, "stderr.txt"))
	if err != nil {
		t.Fatal(err)
	}
	cmd.Stdout, cmd.Stderr = out, out
	cmd.Dir = s.Dir
	cmd.Env = append(os.Environ(), s.Env...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	t.Cleanup(func() {
		syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		time.Sleep(200 * time.Millisecond) // the killed children are gone for good
	})
	f.Close()
	l.Close()
	out.Close()
	if g := waitGreeting(addr, 20*time.Second); !strings.HasPrefix(g, "20") {
		t.Fatalf("greeting on the listener passed as descriptor %d: %q", fd, g)
	}

	stop := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				if c, err := net.DialTimeout("tcp", addr, 200*time.Millisecond); err == nil {
					c.Close()
				}
			}
		}()
	}
	time.Sleep(300 * time.Millisecond)
	exitOut, _ := s.TryRun("drcmd", "exit")
	var werr error
	select {
	case werr = <-done:
	case <-time.After(30 * time.Second):
		t.Errorf("dreaderd still runs 30 s after drcmd exit")
	}
	close(stop)
	wg.Wait()
	b, _ := os.ReadFile(filepath.Join(s.Dir, "stderr.txt"))
	logs := string(b) + generalLog(s)
	t.Logf("drcmd exit: %q; dreaderd ended: %v", strings.TrimSpace(exitOut), werr)
	if strings.Contains(logs, "select returned unknown descriptor") {
		var lines []string
		for _, ln := range strings.Split(logs, "\n") {
			if strings.Contains(ln, "unknown descriptor") {
				lines = append(lines, ln)
			}
		}
		t.Errorf("the main process ended in fatal() during drcmd exit (status %v): %s", werr, strings.Join(lines, "\n"))
	}
}

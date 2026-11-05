package e2e

import (
	"context"
	"github.com/go-while/nntp-testkit/fakeserver"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// Pipe mode must not discard entries left by a previous run when it saves
// articles that the current peer has not accepted.
func TestDnewslinkPipePreservesPreviousQueue(t *testing.T) {
	s := NewSite(t, SiteOptions{NoStart: true})
	dnlSpool(t, s)
	srv := fakeserver.New(t, fakeserver.Options{Greeting: "200 fake", Handler: fakeserver.LineHandler(func(_ *fakeserver.Conn, line string) ([]string, bool) {
		line = strings.ToLower(strings.TrimSpace(line))
		switch {
		case strings.HasPrefix(line, "mode stream"):
			return []string{"203 stream ok"}, false
		case strings.HasPrefix(line, "check "):
			return []string{"400 going away"}, true
		}
		return nil, false
	})})
	port := srv.Port()
	old := "D.00000001/B.0001 <old@e2e.test> 0,100\n"
	newEntry := "D.00000001/B.0001 <new@e2e.test> 100,100\n"
	path := filepath.Join(s.Dir, "dqueue", "out.S00000")
	if err := os.WriteFile(path, []byte(old), 0o640); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, filepath.Join(s.Bin, "dnewslink"), "-p", "-N", "1", "-S", "0", "-b", "out.S%05d", "-h", "127.0.0.1", "-P", strconv.Itoa(port), "-t", "5")
	cmd.Dir = s.Dir
	cmd.Env = append(os.Environ(), s.Env...)
	cmd.Stdin = strings.NewReader(newEntry)
	out, err := cmd.CombinedOutput()
	t.Logf("dnewslink: err=%v\n%s", err, out)
	if ctx.Err() != nil {
		t.Fatal(ctx.Err())
	}
	if err != nil {
		t.Fatalf("dnewslink: %v\n%s", err, out)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"<old@e2e.test>", "<new@e2e.test>"} {
		if n := strings.Count(string(got), id); n != 1 {
			t.Errorf("%s occurs %d times, want once; queue: %q", id, n, got)
		}
	}
}

// A regular batch worker can replace the queue while a pipe worker waits for
// its lock. The pipe worker must append to the current pathname, not an
// unlinked old inode.
func TestDnewslinkPipeQueueReplacedWhileWaiting(t *testing.T) {
	s := NewSite(t, SiteOptions{NoStart: true})
	dnlSpool(t, s)
	srv := fakeserver.New(t, fakeserver.Options{Greeting: "200 fake", Handler: fakeserver.LineHandler(func(_ *fakeserver.Conn, line string) ([]string, bool) {
		line = strings.ToLower(strings.TrimSpace(line))
		switch {
		case strings.HasPrefix(line, "mode stream"):
			return []string{"203 stream ok"}, false
		case strings.HasPrefix(line, "check "):
			return []string{"400 going away"}, true
		}
		return nil, false
	})})
	port := srv.Port()
	path := filepath.Join(s.Dir, "dqueue", "out.S00000")
	if err := os.WriteFile(path, []byte("old queue\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	f, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	lk := syscall.Flock_t{Type: syscall.F_WRLCK, Whence: 0, Start: 0, Len: 0}
	if err := syscall.FcntlFlock(f.Fd(), syscall.F_SETLK, &lk); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, filepath.Join(s.Bin, "dnewslink"), "-p", "-N", "1", "-S", "0", "-b", "out.S%05d", "-h", "127.0.0.1", "-P", strconv.Itoa(port), "-t", "5")
	cmd.Dir = s.Dir
	cmd.Env = append(os.Environ(), s.Env...)
	cmd.Stdin = strings.NewReader("D.00000001/B.0001 <new@e2e.test> 100,100\n")
	var out strings.Builder
	cmd.Stdout, cmd.Stderr = &out, &out
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	if err := waitLockWaiter(cmd.Process.Pid, done); err != nil {
		t.Fatalf("dnewslink did not wait for the queue lock: %v\n%s", err, out.String())
	}
	replacement := path + ".replacement"
	if err := os.WriteFile(replacement, []byte("replacement queue\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(replacement, path); err != nil {
		t.Fatal(err)
	}
	lk.Type = syscall.F_UNLCK
	if err := syscall.FcntlFlock(f.Fd(), syscall.F_SETLK, &lk); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatalf("dnewslink: %v\n%s", err, out.String())
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(got), "replacement queue\n") || strings.Count(string(got), "<new@e2e.test>") != 1 {
		t.Errorf("replacement queue lost an entry: %q\n%s", got, out.String())
	}
}

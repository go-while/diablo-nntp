package e2e

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	testkit "github.com/go-while/nntp-testkit"
)

// outBuffer collects a command's stdout and stderr while the test reads them.
type outBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (o *outBuffer) Write(p []byte) (int, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.b.Write(p)
}

func (o *outBuffer) String() string {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.b.String()
}

// TestDlockhistoryAllGrows: dlockhistory -a keeps a growing history file locked. A small
// append leaves the lock alone; growth by 1/64 locks the larger file (the old lock is held
// until then); a replaced file is locked anew. Before, every size change, i.e. each article
// the feeder adds, unlocked and relocked the whole file on every check (default 5 s).
func TestDlockhistoryAllGrows(t *testing.T) {
	s := NewSite(t, SiteOptions{NoStart: true})
	// -a maps the file as it is, so any 16 KiB file will do; small enough for the
	// usual RLIMIT_MEMLOCK of an unprivileged user, even with two mappings.
	hist := filepath.Join(s.Dir, "dhistory.lock")
	if err := os.WriteFile(hist, make([]byte, 16384), 0644); err != nil {
		t.Fatal(err)
	}
	out := &outBuffer{}
	cmd := exec.Command(filepath.Join(s.Bin, "dlockhistory"), "-a", "-f", "-s", "1", hist)
	cmd.Env = s.Env
	cmd.Stdout, cmd.Stderr = out, out
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	defer func() {
		_ = cmd.Process.Kill()
		<-done
	}()
	exited := func() bool {
		select {
		case err := <-done:
			done <- err
			return true
		default:
			return false
		}
	}
	waitOut := func(what string) {
		t.Helper()
		testkit.WaitFor(t, 10*time.Second, what, func() bool {
			return strings.Contains(out.String(), what) || exited()
		})
		if !strings.Contains(out.String(), what) {
			if strings.Contains(out.String(), "mlock") {
				t.Skipf("mlock refused to this user here: %s", out.String())
			}
			t.Fatalf("dlockhistory exited without %q:\n%s", what, out.String())
		}
	}
	appendBytes := func(n int) {
		t.Helper()
		f, err := os.OpenFile(hist, os.O_WRONLY|os.O_APPEND, 0)
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		if _, err := f.Write(make([]byte, n)); err != nil {
			t.Fatal(err)
		}
	}

	waitOut("Locked 16384 bytes")

	// 200 bytes is less than 16384/64: nothing is unlocked or locked again.
	appendBytes(200)
	time.Sleep(2500 * time.Millisecond)
	if got := out.String(); strings.Count(got, "Locked ") != 1 || strings.Contains(got, "New history") {
		t.Fatalf("after a small append:\n%s\nwant the first lock only", got)
	}

	// 300 bytes in all reaches 1/64: the larger file is locked, still the same history.
	appendBytes(100)
	waitOut("Locked 16684 bytes")
	if strings.Contains(out.String(), "New history") {
		t.Fatalf("growth reported as a new history:\n%s", out.String())
	}

	// A replaced file (as after dhisexpire) is a new history, even when it is shorter.
	if err := os.WriteFile(hist+".new", make([]byte, 8192), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(hist+".new", hist); err != nil {
		t.Fatal(err)
	}
	waitOut("Locked 8192 bytes")
	waitOut("New history")
	if exited() {
		t.Fatalf("dlockhistory exited:\n%s", out.String())
	}
}

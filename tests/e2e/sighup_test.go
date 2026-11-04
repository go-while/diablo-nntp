package e2e

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// TestFeederChildSighupAfterQuit: a feeder child that gets SIGHUP (as `dicmd exit` and
// `dicmd pause` send to every child) while it is logging its session after QUIT still
// exits. Before, the child's SIGHUP handler did the cleanup itself (fflush, LogSession
// through syslog, exit) and could wait forever on a libc lock the interrupted code held:
// the child stayed, with its client socket and dhistory open. Many rounds, because the
// window is small. Linux only (finds the children through /proc).
func TestFeederChildSighupAfterQuit(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("finds the feeder children through /proc")
	}
	s := NewSite(t, SiteOptions{})
	master := s.Proc.Pid()
	children := func() []int {
		return slices.DeleteFunc(sitePids(s, "diablo"), func(pid int) bool { return pid == master })
	}
	rounds := 400
	if n, err := strconv.Atoi(os.Getenv("E2E_SIGHUP_ROUNDS")); err == nil && n > 0 {
		rounds = n
	}
	for range rounds {
		p := s.Peer(t)
		p.ExpectCmd(205, "QUIT")
		for _, pid := range children() {
			_ = syscall.Kill(pid, syscall.SIGHUP)
		}
		p.Close()
	}

	var left []int
	deadline := time.Now().Add(10 * time.Second)
	for {
		if left = children(); len(left) == 0 {
			break
		}
		if time.Now().After(deadline) {
			var b strings.Builder
			for _, pid := range left {
				st, _ := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "status"))
				wchan, _ := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "wchan"))
				blk := ""
				for _, l := range strings.Split(string(st), "\n") {
					if strings.HasPrefix(l, "SigBlk:") || strings.HasPrefix(l, "State:") {
						blk += " " + strings.Join(strings.Fields(l), " ")
					}
				}
				fmt.Fprintf(&b, "\n  pid %d: wchan %s%s", pid, wchan, blk)
			}
			t.Fatalf("%d of %d feeder children still there 10s after QUIT and SIGHUP:%s", len(left), rounds, b.String())
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// TestFeederMasterSigtermEndsSessions: SIGTERM to the master ends an open feed session and the
// master exits, as after "dicmd exit". Before, the master's handler set Exiting before it
// checked it, so it never sent SIGHUP to its children and waited for the session to end on
// its own: a long-lived incoming feed kept the old master running.
func TestFeederMasterSigtermEndsSessions(t *testing.T) {
	s := NewSite(t, SiteOptions{})
	p := s.Peer(t)
	defer p.Close()
	p.ExpectCmd(203, "MODE STREAM")

	if err := syscall.Kill(s.Proc.Pid(), syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	select {
	case <-s.Proc.Done():
	case <-time.After(10 * time.Second):
		t.Fatalf("diablo still running 10s after SIGTERM with a feed session open")
	}
}

// TestFeederPidFileOfNewerMaster: a master that exits removes diablo.pid only while it still
// names that master. Before, an old master that exited after a new one had started removed
// the new master's pid file.
func TestFeederPidFileOfNewerMaster(t *testing.T) {
	s := NewSite(t, SiteOptions{})
	pidFile := filepath.Join(s.Dir, "log", "diablo.pid")
	b, err := os.ReadFile(pidFile)
	if err != nil {
		t.Fatalf("no diablo.pid after start: %v", err)
	}
	if got := strings.TrimSpace(string(b)); got != strconv.Itoa(s.Proc.Pid()) {
		t.Fatalf("diablo.pid %q, want %d", got, s.Proc.Pid())
	}
	const newer = "999999\n" // as written by a master started after this one
	s.WriteFile(t, "log/diablo.pid", newer)

	s.Dicmd(t, "exit")
	select {
	case <-s.Proc.Done():
	case <-time.After(10 * time.Second):
		t.Fatalf("diablo still running 10s after dicmd exit")
	}
	if b, err := os.ReadFile(pidFile); err != nil || string(b) != newer {
		t.Errorf("diablo.pid after the old master exited: %q, %v; want %q", b, err, newer)
	}
}

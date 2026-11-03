package e2e

import (
	"bufio"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// TestControlConnectionNotInherited: "exit" on the control socket keeps the connection
// until diablo is done, and diablo runs path_shutdown_cleanup before that. The cleanup
// program must not inherit the connection: here it leaves a sleep running in the
// background, and the client must still see the connection close when diablo exits.
// Before, the sleep held a copy and the client waited for it (a client that reads to EOF
// instead of to the "." line hung for as long as the program ran).
func TestControlConnectionNotInherited(t *testing.T) {
	dir := t.TempDir()
	pidFile := filepath.Join(dir, "sleep.pid")
	script := filepath.Join(dir, "cleanup.sh")
	body := "#!/bin/sh\nsleep 30 </dev/null >/dev/null 2>&1 &\necho $! > " + pidFile + "\nexit 0\n"
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if b, err := os.ReadFile(pidFile); err == nil {
			if pid, err := strconv.Atoi(strings.TrimSpace(string(b))); err == nil {
				syscall.Kill(pid, syscall.SIGKILL)
			}
		}
	})
	s := NewSite(t, SiteOptions{Config: map[string]string{"path_shutdown_cleanup": script}})

	c, err := net.Dial("unix", filepath.Join(s.Dir, "run", ".diablosock"))
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if _, err := io.WriteString(c, "exit\n"); err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	c.SetReadDeadline(start.Add(15 * time.Second))
	var lines []string
	sc := bufio.NewScanner(c)
	for sc.Scan() {
		lines = append(lines, sc.Text())
	}
	if err := sc.Err(); err != nil {
		t.Fatalf("control connection not closed %.0fs after exit (%v); got %q", time.Since(start).Seconds(), err, lines)
	}
	if len(lines) == 0 || lines[len(lines)-1] != "." {
		t.Errorf("exit reply: %q, want it to end with the \".\" line", lines)
	}
	if _, err := os.Stat(pidFile); err != nil {
		t.Errorf("the shutdown cleanup did not run: %v", err)
	}
}

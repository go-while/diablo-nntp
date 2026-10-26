package e2e

import (
	"bufio"
	"context"
	"errors"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// oneLineServer answers on 127.0.0.1 like an NNTP server whose commands all have one-line
// replies: 200 greeting, 111 for DATE, 211 for GROUP, 205 for QUIT, 500 otherwise.
func oneLineServer(t *testing.T) (port int) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				c.Write([]byte("200 fake server ready\r\n"))
				r := bufio.NewReader(c)
				for {
					line, err := r.ReadString('\n')
					if err != nil {
						return
					}
					cmd := strings.ToUpper(strings.Fields(line + " x")[0])
					switch cmd {
					case "DATE":
						c.Write([]byte("111 20261026120000\r\n"))
					case "GROUP":
						c.Write([]byte("211 1 1 1 e2e.test\r\n"))
					case "QUIT":
						c.Write([]byte("205 bye\r\n"))
						return
					default:
						c.Write([]byte("500 what?\r\n"))
					}
				}
			}(c)
		}
	}()
	return ln.Addr().(*net.TCPAddr).Port
}

// TestDclientOneLineReply: dclient returns after commands with a one-line reply, given on
// the command line or on stdin. Before the fix it waited for a "." line after every reply
// and hung after DATE or GROUP.
func TestDclientOneLineReply(t *testing.T) {
	dclient := filepath.Join(bin(t), "dclient")
	if _, err := os.Stat(dclient); err != nil {
		t.Skipf("no dclient in %s", bin(t))
	}
	port := strconv.Itoa(oneLineServer(t))
	run := func(stdin string, args ...string) string {
		t.Helper()
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, dclient, append([]string{"-p", port, "127.0.0.1"}, args...)...)
		cmd.Stdin = strings.NewReader(stdin)
		out, err := cmd.CombinedOutput()
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			t.Fatalf("dclient %v hung after a one-line reply; output so far:\n%s", args, out)
		}
		if err != nil {
			t.Fatalf("dclient %v: %v\n%s", args, err, out)
		}
		return string(out)
	}
	if out := run("", "DATE"); !strings.Contains(out, "111 20261026120000") {
		t.Errorf("dclient DATE: no 111 reply in %q", out)
	}
	out := run("GROUP e2e.test\nDATE\n")
	if !strings.Contains(out, "211 1 1 1 e2e.test") || !strings.Contains(out, "111 ") {
		t.Errorf("dclient with GROUP and DATE on stdin: %q", out)
	}
}

// TestPlockExitStatus: plock returns the exit status of the command it runs (before: 0).
func TestPlockExitStatus(t *testing.T) {
	plock := filepath.Join(bin(t), "plock")
	if _, err := os.Stat(plock); err != nil {
		t.Skipf("no plock in %s", bin(t))
	}
	lock := filepath.Join(t.TempDir(), "lock")
	for want, cmd := range map[int]string{0: "true", 3: "exit 3"} {
		err := exec.Command(plock, lock, cmd).Run()
		got := 0
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			got = ee.ExitCode()
		} else if err != nil {
			t.Fatalf("plock %q: %v", cmd, err)
		}
		if got != want {
			t.Errorf("plock %q: exit status %d, want %d", cmd, got, want)
		}
	}
}

// TestDhisbenchNeedsScratchHistory: dhisbench refuses to add its made-up entries to the
// configured dhistory; with -f it adds to the given file (before: it wrote into dhistory).
func TestDhisbenchNeedsScratchHistory(t *testing.T) {
	s := NewSite(t, SiteOptions{NoStart: true})
	out, err := s.TryRun("dhisbench", "-af", "1", "-ac", "10")
	if err == nil || !strings.Contains(out, "scratch history file") {
		t.Errorf("dhisbench adds without -f: err=%v, output %q; want a refusal", err, out)
	}
	if _, err := os.Stat(filepath.Join(s.Dir, "dhistory")); err == nil {
		t.Errorf("dhisbench created or wrote the site's dhistory")
	}
	scratch := filepath.Join(t.TempDir(), "dhistory.bench")
	out = s.Run(t, "dhisbench", "-af", "1", "-ac", "10", "-f", scratch, "-m", scratch+".map")
	if !strings.Contains(out, "adds per second") {
		t.Errorf("dhisbench -f: %q", out)
	}
}

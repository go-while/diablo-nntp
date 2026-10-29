package e2e

import (
	"bufio"
	"context"
	"net"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestReaderdefNumbersValidated (D-161): a readerdef number that is not all digits (or is
// out of range) makes the access file invalid: it is not installed, so a dreaderd that has
// no other access cache gives no service. atoi() made "abc" 0 and "1x" 1, and dreaderd
// served with limits the admin never wrote.
func TestReaderdefNumbersValidated(t *testing.T) {
	for _, bad := range []string{"maxconnperhost 1x", "idletimeout abc", "maxconn 99999999999", "bytelimit -5"} {
		t.Run(strings.ReplaceAll(bad, " ", "_"), func(t *testing.T) {
			access := strings.Replace(DefaultReaderAccess, "readerdef rp\n", "readerdef rp\n    "+bad+"\n", 1)
			s := NewReaderSite(t, ReaderOptions{SpoolAddr: "127.0.0.1:1", Access: access, NoStart: true})
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			cmd := exec.CommandContext(ctx, filepath.Join(s.Bin, "dreaderd"), "-d", "-p", PathHost, "-h", PathHost, "-P", "[127.0.0.1]:"+s.Addr[strings.LastIndex(s.Addr, ":")+1:])
			cmd.Dir = s.Dir
			cmd.Env = append(cmd.Environ(), s.Env...)
			if err := cmd.Start(); err != nil {
				t.Fatal(err)
			}
			defer func() { cancel(); cmd.Wait() }()

			// the greeting of a reader that got its access definitions is 200 or 201
			var greeting string
			deadline := time.Now().Add(4 * time.Second)
			for time.Now().Before(deadline) {
				conn, err := net.DialTimeout("tcp", s.Addr, time.Second)
				if err != nil {
					time.Sleep(100 * time.Millisecond)
					continue
				}
				conn.SetReadDeadline(time.Now().Add(time.Second))
				greeting, _ = bufio.NewReader(conn).ReadString('\n')
				conn.Close()
				if greeting != "" {
					break
				}
			}
			if strings.HasPrefix(greeting, "200") || strings.HasPrefix(greeting, "201") {
				t.Errorf("dreaderd with %q in dreader.access gave service: %q", bad, strings.TrimSpace(greeting))
			}
		})
	}
}

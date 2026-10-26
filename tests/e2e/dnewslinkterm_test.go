package e2e

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/go-while/nntp-testkit/fakeserver"
)

// TestDnewslinkTermStalledPeer: dnewslink exits soon after SIGTERM while it waits for a peer
// that has stopped answering, at the greeting and in the middle of a streaming session.
// Before, a signal only interrupted the poll() of the wait; dnewslink then polled again for
// the full -T timeout (600 s by default) and only saw the signal afterwards, so a SIGTERM'd
// worker on a hung peer looked unkillable. The unsent articles stay queued.
func TestDnewslinkTermStalledPeer(t *testing.T) {
	cases := []struct {
		name string
		opts fakeserver.Options
	}{
		{"greeting", fakeserver.Options{NoGreeting: true}},
		{"check", fakeserver.Options{Handler: func(c *fakeserver.Conn) {
			// Stream mode yes, then never answer CHECK or TAKETHIS.
			for {
				l, err := c.ReadLine()
				if err != nil {
					return
				}
				if strings.EqualFold(l, "MODE STREAM") {
					_ = c.WriteLines("203 StreamOK")
				}
			}
		}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			peer := fakeserver.New(t, tc.opts)
			s := NewSite(t, SiteOptions{Newsfeeds: OutNewsfeeds})
			ids := s.Inject(t, "e2e.test", 2)
			s.waitQueue(t, OutLabel, 2)

			cmd := exec.Command(filepath.Join(s.Bin, "dnewslink"),
				"-b", OutLabel, "-h", "127.0.0.1", "-P", strconv.Itoa(peer.Port()))
			cmd.Dir = s.Dir
			cmd.Env = append(os.Environ(), s.Env...)
			var out bytes.Buffer
			cmd.Stdout, cmd.Stderr = &out, &out
			if err := cmd.Start(); err != nil {
				t.Fatal(err)
			}
			done := make(chan struct{})
			go func() { _ = cmd.Wait(); close(done) }()
			t.Cleanup(func() {
				_ = cmd.Process.Kill()
				<-done
			})

			peer.WaitOpen(t, 1, 10*time.Second)
			if tc.name == "check" {
				waitFor(t, 10*time.Second, "a CHECK or TAKETHIS", func() bool {
					return peer.Count("CHECK")+peer.Count("TAKETHIS") > 0
				})
			}
			time.Sleep(500 * time.Millisecond) // dnewslink is in its poll() now

			start := time.Now()
			if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
				t.Fatal(err)
			}
			select {
			case <-done:
				t.Logf("dnewslink exited %v after SIGTERM", time.Since(start).Round(time.Millisecond))
			case <-time.After(5 * time.Second):
				t.Fatalf("dnewslink still running 5s after SIGTERM while the peer is stalled\n%s", out.String())
			}

			// Nothing was accepted, so both articles must still be queued.
			q := strings.Join(s.queue(t, OutLabel), "\n")
			for _, id := range ids {
				if !strings.Contains(q, id) {
					t.Errorf("%s no longer queued after the terminated dnewslink:\n%s", id, q)
				}
			}
		})
	}
}

package e2e

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/go-while/nntp-testkit/fakeserver"
)

// TestDeepRevDnewslinkRefileOffset (L3-05): an entry that is pending when the connection ends
// is written back to the queue file. That line was printed with the spool offset cut to an
// int, while every other writer of queue lines prints the whole offset: an offset of
// 3000000000 came back as -1294967296. (Spool files stop at 1 GiB today, so this is a line
// from a queue file made by hand or by a later version.)
func TestDeepRevDnewslinkRefileOffset(t *testing.T) {
	s := NewSite(t, SiteOptions{NoStart: true})
	dnlSpool(t, s)
	var checked atomic.Bool
	srv := fakeserver.New(t, fakeserver.Options{Greeting: "200 fake", Handler: fakeserver.LineHandler(
		func(_ *fakeserver.Conn, line string) ([]string, bool) {
			line = strings.ToLower(strings.TrimSpace(line))
			switch {
			case strings.HasPrefix(line, "mode stream"):
				return []string{"203 stream ok"}, false
			case strings.HasPrefix(line, "check "):
				checked.Store(true)
				return []string{"400 going away"}, true
			}
			return nil, false
		})})

	const id, at = "<far@e2e.test>", "3000000000,100"
	queue := filepath.Join(s.Dir, "dqueue", "out.S00000")
	if err := os.WriteFile(queue, []byte("D.00000001/B.0001 "+id+" "+at+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	before, err := os.Stat(queue)
	if err != nil {
		t.Fatal(err)
	}

	out, err := s.TryRun("dnewslink", "-N", "1", "-S", "0", "-b", "out.S%05d", "-h", "127.0.0.1",
		"-P", strconv.Itoa(srv.Port()), "-t", "5")
	t.Logf("dnewslink: err=%v\n%s", err, out)
	left := dnlQueue(t, s, "out.S00000")

	// the queue that is there now must be the rewrite, not the file the test wrote
	if !checked.Load() {
		t.Fatalf("the scenario did not develop: dnewslink did not offer the entry to the peer")
	}
	if after, err := os.Stat(queue); err != nil || os.SameFile(before, after) {
		t.Fatalf("the scenario did not develop: dnewslink did not rewrite the queue file (%v)", err)
	}
	if !strings.Contains(left, id) {
		t.Fatalf("the scenario did not develop: the pending entry is not back in the queue file:\n%s", left)
	}
	if f := strings.Fields(left); len(f) < 3 || f[2] != at {
		t.Errorf("the entry came back with another offset,size (want %s):\n%s", at, left)
	}
}

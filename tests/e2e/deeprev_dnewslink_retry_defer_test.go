package e2e

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/go-while/nntp-testkit/fakeserver"
)

// TestDeepRevDnewslinkRetryBeforeDefer (NEW-2, the rest of L3-01): after a remote EOF
// dnewslink keeps the pending stream entries in their slots, marked for a retry, and reads
// them back one by one. A new entry is put into the slot behind the pending ones, which is
// free only when the retries are read back first. While dnewslink was sending its deferred
// entries (431 earlier) again, the next deferred entry was taken before the retries and went
// into slot 0, over the entry kept there: lost, one per deferred entry still waiting, and the
// retry count stayed too high for the rest of the process.
//
// The peer of this test refuses the first queued entry and defers the other 24, then gets
// those offered again; it does
// not answer and closes the connection at the 16th of those CHECKs (window full, dnewslink
// waits; eight deferred entries are still to come). On later connections it refuses every
// CHECK. Every queued id must be offered on a later connection or still be in the queue file.
func TestDeepRevDnewslinkRetryBeforeDefer(t *testing.T) {
	const total = 24
	s := NewSite(t, SiteOptions{NoStart: true})
	dnlSpool(t, s)

	var (
		mu      sync.Mutex
		first   int                 // CHECK lines on connection 1
		offered = map[string]bool{} // CHECKed on a later connection
	)
	srv := fakeserver.New(t, fakeserver.Options{Greeting: "200 fake", Handler: fakeserver.LineHandler(
		func(c *fakeserver.Conn, line string) ([]string, bool) {
			f := strings.Fields(line)
			if len(f) == 0 {
				return nil, false
			}
			switch strings.ToLower(f[0]) {
			case "mode":
				return []string{"203 stream ok"}, false
			case "quit":
				return []string{"205 bye"}, true
			case "check":
				if len(f) < 2 {
					return nil, false
				}
				mu.Lock()
				defer mu.Unlock()
				if c.ID() == 1 {
					first++
					if first == 1 {
						return []string{"438 " + f[1]}, false // dnewslink reconnects only after an answer
					}
					if first <= 1+total {
						return []string{"431 " + f[1]}, false
					}
					return nil, first >= 1+total+16 // no answer; go away with the window full
				}
				offered[f[1]] = true
				return []string{"438 " + f[1]}, false
			}
			return nil, false
		})})

	var ids []string
	var q strings.Builder
	q.WriteString("D.00000001/B.0001 <y-first@e2e.test> 0,100\n")
	for i := 0; i < total; i++ {
		id := fmt.Sprintf("<y%02d@e2e.test>", i)
		ids = append(ids, id)
		fmt.Fprintf(&q, "D.00000001/B.0001 %s %d,100\n", id, i*100)
	}
	if err := os.WriteFile(filepath.Join(s.Dir, "dqueue", "out.S00000"), []byte(q.String()), 0o644); err != nil {
		t.Fatal(err)
	}

	out, err := s.TryRun("dnewslink", "-N", "1", "-S", "0", "-b", "out.S%05d", "-h", "127.0.0.1",
		"-P", strconv.Itoa(srv.Port()), "-t", "5")
	t.Logf("dnewslink: err=%v\n%s", err, out)
	left := dnlQueue(t, s, "out.S00000")

	mu.Lock()
	defer mu.Unlock()
	if first != 1+total+16 || srv.Accepted() < 2 {
		t.Fatalf("the scenario did not develop: connection 1 got %d CHECKs (want %d: one refused, %d deferred, 16 of them offered again), %d connections (want 2 or more)\nqueue file now:\n%s",
			first, 1+total+16, total, srv.Accepted(), left)
	}
	var lost []string
	for _, id := range ids {
		if !offered[id] && !strings.Contains(left, id) {
			lost = append(lost, id)
		}
	}
	if len(lost) > 0 {
		t.Errorf("deferred entries that dnewslink neither offered again after the reconnect nor left in the queue file: %q\n"+
			"offered later: %d, queue file now:\n%s", lost, len(offered), left)
	}
}

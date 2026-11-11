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

// TestDeepRevDnewslinkReconnectFullWindow (L3-01): dnewslink streams up to 16 CHECKs ahead
// of the peer's answers. When the connection ends while it waits for an answer with the
// window full, it marks the 16 pending entries for a retry, reconnects, and then offered the
// queue line it was holding once more, although that line was one of the 16. The second
// offer was put into stream slot 0, which still held another entry marked for retry: that
// entry was overwritten and is neither sent nor written back to the queue file. One queued
// article is lost per such reconnect, silently. A peer that stops answering and then drops
// the connection produces exactly this state.
//
// The peer of this test refuses the first six CHECKs (438), then stops answering and closes
// the connection when the 22nd CHECK arrives: by then dnewslink has read the six answers,
// refilled its window of 16 and waits. On the next connections every CHECK is refused. Every
// queued id must be refused on the first connection, offered on a later one, or still be in
// the queue file.
func TestDeepRevDnewslinkReconnectFullWindow(t *testing.T) {
	const total = 40
	s := NewSite(t, SiteOptions{NoStart: true})
	dnlSpool(t, s)

	var (
		mu      sync.Mutex
		first   int                 // CHECK lines on connection 1
		refused = map[string]bool{} // answered 438 on connection 1
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
					if first <= 6 {
						refused[f[1]] = true
						return []string{"438 " + f[1]}, false
					}
					return nil, first >= 22 // no answer; go away at the 22nd
				}
				offered[f[1]] = true
				return []string{"438 " + f[1]}, false
			}
			return nil, false
		})})

	// Short ids: dnewslink also stops reading ahead at 896 bytes of pending Message-IDs, and
	// the window of 16 must fill first.
	var ids []string
	var q strings.Builder
	for i := 0; i < total; i++ {
		id := fmt.Sprintf("<z%02d@e2e.test>", i)
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
	if first != 22 || srv.Accepted() < 2 {
		t.Fatalf("the scenario did not develop: connection 1 got %d CHECKs (want 22: 16, six answers read, six more), %d connections (want 2 or more)\nqueue file now:\n%s",
			first, srv.Accepted(), left)
	}
	var lost []string
	for _, id := range ids {
		if !refused[id] && !offered[id] && !strings.Contains(left, id) {
			lost = append(lost, id)
		}
	}
	if len(lost) > 0 {
		t.Errorf("queued entries that dnewslink neither offered again after the reconnect nor left in the queue file: %q\n"+
			"refused on connection 1: %d, offered later: %d, queue file now:\n%s",
			lost, len(refused), len(offered), left)
	}
}

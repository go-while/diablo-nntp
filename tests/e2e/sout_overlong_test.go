package e2e

import (
	"fmt"
	"strings"
	"testing"
)

// TestSoutOverlongLine (D-114): a feeder child reports each accepted article to the master as
// one SOUT line on the control pipe, which the master reads with egets() into a buffer of one
// page (about 4 KB). Here the Newsgroups and Path lines are about 2.5 KB and 1.8 KB, so the
// SOUT line is longer than the buffer. The master used to lose the first part of such a line,
// saw a tail that does not start with SOUT and never queued the article for the outgoing label.
// The long article must reach dqueue/peer, and so must the short one sent after it.
func TestSoutOverlongLine(t *testing.T) {
	// the peer feeds in under "peer"; articles are queued for "out" (an article is never fed
	// back to the label it came from)
	s := NewSite(t, SiteOptions{Newsfeeds: GlobalLabel + `label peer
    inhost ` + PeerIP + `
    alias e2e.peer
    addgroup *
end
label out
    hostname 127.0.0.1
    addgroup *
end
`})
	var groups []string
	for i := 0; i < 150; i++ {
		groups = append(groups, fmt.Sprintf("e2e.overlong.g%03d", i))
	}
	longID := msgID(t)
	long := newArticle(longID, strings.Join(groups, ","), "overlong SOUT line", "body")
	long[0] = "Path: e2e.peer!" + strings.Repeat("hop.example.invalid!", 90) + "not-for-mail"
	shortID := msgID(t)
	short := newArticle(shortID, "e2e.test", "short SOUT line", "body")

	p := s.Peer(t)
	for _, a := range []struct {
		id    string
		lines []string
	}{{longID, long}, {shortID, short}} {
		expect(t, p, 335, "IHAVE %s", a.id)
		send(t, p, wire(a.lines))
		status(t, p, 235, "IHAVE "+a.id)
	}
	expect(t, p, 205, "QUIT")

	q := s.waitQueue(t, "out", 2)
	joined := strings.Join(q, "\n")
	for _, id := range []string{longID, shortID} {
		if !strings.Contains(joined, id) {
			t.Errorf("dqueue/out = %q, want %s (its SOUT line is longer than the control pipe buffer)", q, id)
		}
	}
}

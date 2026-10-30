package e2e

// GHSA-3j76-xmx3-7q86: the feeder master's queue line must not take its
// length from a field that a feed peer can inflate.
//
// The article below has a 2488-byte Path and three TABs in Distribution, so
// the child-to-master SOUT line puts the Path in the cSize slot: the old
// master computed the queue-line length without cSize and sprintf()ed past
// its 4096-byte page.  Thirty short articles first fill that page.
//
// The fix may either reject the tabbed article (437, the DistributionTab
// rule) or sanitise the field and queue the article with the real cSize.  The
// test accepts both, and requires that the master is still there and accepts
// another article afterwards.

import (
	"strings"
	"testing"
)

func TestFeederWriteFeedOutQueueLine(t *testing.T) {
	newsfeeds := GlobalLabel + `label peer
    inhost ` + PeerIP + `
    alias e2e.peer
    addgroup *
end
label out
    hostname 127.0.0.1
    addgroup *
end
`
	s := NewSite(t, SiteOptions{Newsfeeds: newsfeeds})
	p := s.Peer(t)

	// Fill the master's fe_Buf (4096 bytes) with ordinary queue lines.
	for i := 0; i < 30; i++ {
		id := msgID(t)
		expect(t, p, 335, "IHAVE %s", id)
		send(t, p, wire(newArticle(id, "e2e.test", "fill", strings.Repeat("C", 200))))
		status(t, p, 235, "IHAVE fill")
	}

	// The shifted field is the Path: 2488 bytes, three TABs in Distribution.
	fire := msgID(t)
	path := strings.Repeat("d", 2488) + "!peer2!peer3"
	art := newArticle(fire, "e2e.test", "fire", "body")
	art[0] = "Path: " + path
	art = append([]string{art[0], "Distribution: world\tE1\tE2\t000000"}, art[1:]...)
	expect(t, p, 335, "IHAVE %s", fire)
	send(t, p, wire(art))
	code, line, err := p.ReadStatus()
	if err != nil {
		t.Fatalf("fire article: %v", err)
	}
	switch code {
	case 437:
		// The tabbed article is refused before it reaches the queue.
	case 235:
		// It was queued: the cSize field must not be the Path.
		s.Dicmd(t, "flush")
		q := s.waitQueue(t, "out", 31)
		var fireLine string
		for _, l := range q {
			if strings.Contains(l, fire) {
				fireLine = l
			}
		}
		if fireLine == "" {
			t.Fatalf("fire article accepted but not in dqueue/out: %q", q)
		}
		for _, f := range strings.Fields(fireLine) {
			if strings.HasPrefix(f, "C") && len(f) > 64 {
				t.Errorf("queue line carries the Path in the cSize field: %q", fireLine)
			}
		}
	default:
		t.Fatalf("fire article: %d %q, want 235 or 437", code, line)
	}

	// The master must still be alive: it has to queue another article.
	probe := msgID(t)
	expect(t, p, 335, "IHAVE %s", probe)
	send(t, p, wire(newArticle(probe, "e2e.test", "probe", "body")))
	status(t, p, 235, "IHAVE probe after the fire article")
	s.Dicmd(t, "flush")
	expect(t, p, 205, "QUIT")
}

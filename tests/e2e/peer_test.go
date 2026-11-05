package e2e

import (
	"strings"
	"testing"

	"github.com/go-while/nntp-testkit/client"
)

// TestPeerIHAVE feeds an article with IHAVE from the peer and offers it again: diablo stores
// it (235), then refuses the duplicate offer (435).
func TestPeerIHAVE(t *testing.T) {
	s := NewSite(t, SiteOptions{})
	p := s.Peer(t)
	if code, err := client.ParseCode(p.Greeting); err != nil || code != 200 {
		t.Fatalf("peer greeting %q, want 200", p.Greeting)
	}
	id := msgID(t)
	p.ExpectCmd(335, "IHAVE %s", id)
	p.MustSend(client.Stuff(newArticle(id, "e2e.test", "via ihave", "hello", ".dot")))
	p.ExpectStatus(235, "IHAVE article")
	p.ExpectCmd(435, "IHAVE %s", id)
	p.ExpectCmd(205, "QUIT")
}

// TestStrangerRefused: a host that no dnewsfeeds label lists gets a 502 permission denial.
func TestStrangerRefused(t *testing.T) {
	s := NewSite(t, SiteOptions{})
	c := s.Stranger(t)
	if !strings.HasPrefix(c.Greeting, "502 ") || !strings.Contains(c.Greeting, "permission denied") {
		t.Fatalf("greeting %q, want 502 ... Transfer permission denied", c.Greeting)
	}
}

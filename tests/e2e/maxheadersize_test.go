package e2e

import (
	"github.com/go-while/nntp-testkit/client"
	"strings"
	"testing"
)

// TestFeederMaxHeaderSize (D-56): an article whose headers are larger than feedermaxheadersize
// is refused, and the next article is still taken. Fails on main: the limit was never applied
// and an article with about 130 KB of headers was accepted with 235 under a limit of 2k.
func TestFeederMaxHeaderSize(t *testing.T) {
	s := NewSite(t, SiteOptions{Config: map[string]string{"feedermaxheadersize": "2k"}})
	p := s.Peer(t)

	big := newArticle(msgID(t), "e2e.test", "big headers", "body")
	var pad []string
	for i := 0; i < 2000; i++ {
		pad = append(pad, "X-Pad: "+strings.Repeat("a", 60))
	}
	art := append(append([]string{}, big[:6]...), pad...)
	art = append(art, big[6:]...)
	id := headerValue(art, "Message-ID")
	p.ExpectCmd(335, "IHAVE %s", id)
	p.MustSend(client.Stuff(art))
	code, line, err := p.ReadStatus()
	if err != nil {
		t.Fatal(err)
	}
	if code/100 == 2 {
		t.Errorf("article with 130 KB of headers accepted under feedermaxheadersize 2k: %d %q", code, line)
	}

	ok := newArticle(msgID(t), "e2e.test", "small headers", "body")
	okID := headerValue(ok, "Message-ID")
	p.ExpectCmd(335, "IHAVE %s", okID)
	p.MustSend(client.Stuff(ok))
	p.ExpectStatus(235, "IHAVE small article")
	p.ExpectCmd(205, "QUIT")
}

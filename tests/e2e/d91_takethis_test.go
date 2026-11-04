package e2e

import (
	"fmt"
	"github.com/go-while/nntp-testkit/client"
	"testing"
)

// TestFeederTakethisNoArg: TAKETHIS without a message-id must return 501 and drain
// the article body so the session stays usable. On main, TAKETHIS with no argument
// passes an empty/bad msgid ("<>") through to LoadArticle and returns 439 (not the
// correct 501); the article body is drained either way. The fix responds 501
// immediately and drains via LoadArticle(noWrite=1). Cross-platform: the wrong
// status code shows on all platforms.
func TestFeederTakethisNoArg(t *testing.T) {
	id := msgID(t)
	art := newArticle(id, "e2e.test", "d91 no-arg takethis", "body line")

	s := NewSite(t, SiteOptions{})
	p := s.Peer(t)

	p.ExpectCmd(203, "MODE STREAM")

	// TAKETHIS with no argument: the article body still follows on the wire.
	if err := p.Send(fmt.Sprintf("TAKETHIS\r\n%s", client.Stuff(art))); err != nil {
		t.Fatal(err)
	}
	p.ExpectStatus(501, "TAKETHIS no-arg")

	// The session must still work: a valid TAKETHIS after the 501 should be accepted.
	id2 := msgID(t)
	art2 := newArticle(id2, "e2e.test", "d91 follow-up", "body line")
	if err := p.Send(fmt.Sprintf("TAKETHIS %s\r\n%s", id2, client.Stuff(art2))); err != nil {
		t.Fatal(err)
	}
	p.ExpectStatus(239, "TAKETHIS valid after no-arg")

	p.ExpectCmd(205, "QUIT")
}

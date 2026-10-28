package e2e

import (
	"fmt"
	"testing"
)

// TestFeederTakethisNoArg: TAKETHIS without a message-id must return 501 and drain
// the article body so the session stays usable. Fails on main: the server reads msgid
// via MsgId(NULL,NULL)="<>" and calls LoadArticle with that id, corrupting the stream.
func TestFeederTakethisNoArg(t *testing.T) {
	id := msgID(t)
	art := newArticle(id, "e2e.test", "d91 no-arg takethis", "body line")

	s := NewSite(t, SiteOptions{})
	p := s.Peer(t)

	expect(t, p, 203, "MODE STREAM")

	// TAKETHIS with no argument: the article body still follows on the wire.
	if err := p.Send(fmt.Sprintf("TAKETHIS\r\n%s", wire(art))); err != nil {
		t.Fatal(err)
	}
	status(t, p, 501, "TAKETHIS no-arg")

	// The session must still work: a valid TAKETHIS after the 501 should be accepted.
	id2 := msgID(t)
	art2 := newArticle(id2, "e2e.test", "d91 follow-up", "body line")
	if err := p.Send(fmt.Sprintf("TAKETHIS %s\r\n%s", id2, wire(art2))); err != nil {
		t.Fatal(err)
	}
	status(t, p, 239, "TAKETHIS valid after no-arg")

	expect(t, p, 205, "QUIT")
}

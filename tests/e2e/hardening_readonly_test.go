package e2e

import (
	"os"
	"testing"
)

// TestFeederReadOnlyTakethis: on a read-only connection TAKETHIS is refused with 480, and the
// article that follows the command is read and thrown away, not run line by line as commands.
// Before the fix every article line got a 500 reply, and a body line that was a command
// (FEEDRSET, FEEDADD, FEEDCOMMIT) was executed.
func TestFeederReadOnlyTakethis(t *testing.T) {
	s := NewSite(t, SiteOptions{Newsfeeds: GlobalLabel + `label peer
    inhost ` + PeerIP + `
    addgroup *
    readonly
end
`})
	p := s.Peer(t)
	id := msgID(t)
	send(t, p, "TAKETHIS "+id+"\r\n"+wire(newArticle(id, "e2e.test", "read-only", "FEEDRSET", "FEEDADD alt.e2e.*", "FEEDCOMMIT")))
	status(t, p, 480, "TAKETHIS on a read-only connection")
	// Nothing but the reply to QUIT may follow.
	expect(t, p, 205, "QUIT")
	if b, err := os.ReadFile(s.Dir + "/feeds/peer"); err == nil {
		t.Errorf("feeds/peer was written from the article body: %q", b)
	}
}

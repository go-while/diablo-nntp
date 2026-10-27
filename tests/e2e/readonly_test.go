package e2e

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestFeederReadOnlyTakethisDiscardsArticle: on a read-only connection (dnewsfeeds
// "readonly yes", or MODE READONLY) diablo refuses a TAKETHIS with one 480 and reads the
// article that follows it as data: the article's lines are not run as commands. The body
// holds FEEDRSET, FEEDADD and FEEDCOMMIT lines, which would rewrite feeds/peer if they were
// run. After the 480 the next command gets its own reply, nothing else is pending, and
// feeds/peer does not exist.
func TestFeederReadOnlyTakethisDiscardsArticle(t *testing.T) {
	readonlyFeeds := GlobalLabel + `label peer
    inhost ` + PeerIP + `
    addgroup *
    readonly yes
end
`
	for _, c := range []struct {
		name      string
		newsfeeds string
		mode      bool // send MODE READONLY first
		greeting  string
	}{
		{"dnewsfeeds readonly", readonlyFeeds, false, "201"},
		{"mode readonly", DefaultNewsfeeds, true, "200"},
	} {
		t.Run(c.name, func(t *testing.T) {
			s := NewSite(t, SiteOptions{Newsfeeds: c.newsfeeds})
			p := s.Peer(t)
			if len(p.Greeting) < 3 || p.Greeting[:3] != c.greeting {
				t.Fatalf("greeting %q, want %s", p.Greeting, c.greeting)
			}
			if c.mode {
				expect(t, p, 250, "MODE READONLY")
			}
			expect(t, p, 203, "MODE STREAM")
			id := msgID(t)
			art := newArticle(id, "e2e.test", "read-only takethis",
				"FEEDRSET", "FEEDADD e2e.injected.*", "FEEDCOMMIT", "MODE HEADFEED")
			send(t, p, "TAKETHIS "+id+"\r\n"+wire(art))
			status(t, p, 480, "TAKETHIS on a read-only connection")
			// The next reply must be the one for this command, not one for an article line.
			expect(t, p, 203, "MODE STREAM")
			if err := p.ExpectNoPending(500 * time.Millisecond); err != nil {
				t.Errorf("after TAKETHIS: %v", err)
			}
			expect(t, p, 205, "QUIT")
			for _, f := range []string{"feeds/peer", "feeds/peer.new"} {
				if _, err := os.Stat(filepath.Join(s.Dir, f)); err == nil {
					t.Errorf("%s exists: the article's FEEDRSET/FEEDCOMMIT lines were run", f)
				}
			}
		})
	}
}

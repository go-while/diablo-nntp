package e2e

import (
	"slices"
	"testing"
	"time"

	"github.com/go-while/nntp-testkit/client"
)

// TestDeepRevSoutTabBeforeCR (L1-02): a feeder session reports each accepted article to the
// diablo master as one TAB-separated "SOUT" line (lib/newsfeed.c, FeedAdd()), so a TAB in the
// Path: or Distribution: value must refuse the article (PathTab, DistributionTab). The check
// in LoadArticle() lets a TAB pass when a CR or LF follows it (a trailing TAB before the line
// end is tolerated), also in the middle of the line, and the copy of the value then drops the
// CR: "x<TAB><CR>y" is stored as "x<TAB>y" and goes into the SOUT line. The master's fields
// shift by one for every such TAB, and the sender chooses what the master takes as the Path
// (feed exclusion by Path), the header-only flag, the article type and the queue flags.
// rejectartswithbarecr, which would refuse these articles, is off by default.
//
// Both articles must be refused: 437 for IHAVE, 439 for TAKETHIS.
func TestDeepRevSoutTabBeforeCR(t *testing.T) {
	for _, c := range []struct {
		name string
		edit func(art []string) []string
		// what the sender gains while the bug is there, for the failure message
		effect string
	}{
		{"path", func(a []string) []string {
			a[0] = "Path: e2e.peer!x\t\r1\t\rffffff!not-for-mail"
			return a
		}, "the queue line of the full article gets the header-only flag \" H\""},
		{"distribution", func(a []string) []string {
			return slices.Insert(a, 1, "Distribution: world\t\re2e.fake")
		}, "the master takes \"e2e.fake\" as the Path and does not queue the article for that feed"},
	} {
		t.Run(c.name, func(t *testing.T) {
			s := NewSite(t, SiteOptions{Newsfeeds: OutNewsfeeds})
			p := s.Peer(t)

			id1 := msgID(t)
			p.ExpectCmd(335, "IHAVE %s", id1)
			p.MustSend(client.Stuff(c.edit(newArticle(id1, "e2e.test", "TAB before CR", "body"))))
			ihave, line1, err := p.ReadStatus()
			if err != nil {
				t.Fatalf("IHAVE %s: %v (%q)", id1, err, line1)
			}

			p.ExpectCmd(203, "MODE STREAM")
			id2 := msgID(t)
			p.MustSend("TAKETHIS " + id2 + "\r\n" +
				client.Stuff(c.edit(newArticle(id2, "e2e.test", "TAB before CR", "body"))))
			takethis, line2, err := p.ReadStatus()
			if err != nil {
				t.Fatalf("TAKETHIS %s: %v (%q)", id2, err, line2)
			}
			p.ExpectCmd(205, "QUIT")

			if ihave != 437 || takethis != 439 {
				// Show what reached the outgoing queue: give the master a moment for the SOUT lines.
				time.Sleep(time.Second)
				t.Errorf("an article with TAB CR in its %s header was not refused: IHAVE %q, TAKETHIS %q; want 437 and 439.\n"+
					"While it is accepted, %s.\ndqueue/%s: %q",
					c.name, line1, line2, c.effect, OutLabel, s.queue(t, OutLabel))
			}
		})
	}
}

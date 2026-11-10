package e2e

import (
	"testing"
	"time"

	"github.com/go-while/nntp-testkit/client"
)

// TestDeepRevEmptyPath (L1-10): a feeder session reports each accepted article to the diablo
// master as one TAB-separated "SOUT" line with the Path: value as one field, and the master
// splits it with strtok(), which takes two TABs in a row as one separator. A Path: header
// without a value ("Path:" or "Path: ") was accepted and gave an empty field, so every field
// behind it moved up by one: the master read the header-only flag as the Path, the article
// type as the header-only flag (000001 = set) and the line count as the article type. The
// article got " H" in every queue and a normal dnewslink then never sent it.
//
// Both articles must be refused: 437 for IHAVE, 439 for TAKETHIS.
func TestDeepRevEmptyPath(t *testing.T) {
	for _, c := range []struct{ name, path string }{
		{"novalue", "Path:"},
		{"blank", "Path: "},
	} {
		t.Run(c.name, func(t *testing.T) {
			s := NewSite(t, SiteOptions{Newsfeeds: OutNewsfeeds})
			p := s.Peer(t)
			edit := func(a []string) []string {
				a[0] = c.path
				return a
			}

			id1 := msgID(t)
			p.ExpectCmd(335, "IHAVE %s", id1)
			p.MustSend(client.Stuff(edit(newArticle(id1, "e2e.test", "empty Path", "body"))))
			ihave, line1, err := p.ReadStatus()
			if err != nil {
				t.Fatalf("IHAVE %s: %v (%q)", id1, err, line1)
			}

			p.ExpectCmd(203, "MODE STREAM")
			id2 := msgID(t)
			p.MustSend("TAKETHIS " + id2 + "\r\n" +
				client.Stuff(edit(newArticle(id2, "e2e.test", "empty Path", "body"))))
			takethis, line2, err := p.ReadStatus()
			if err != nil {
				t.Fatalf("TAKETHIS %s: %v (%q)", id2, err, line2)
			}
			p.ExpectCmd(205, "QUIT")

			if ihave != 437 || takethis != 439 {
				// Show what reached the outgoing queue: give the master a moment for the SOUT lines.
				time.Sleep(time.Second)
				t.Errorf("an article with %q and no value was not refused: IHAVE %q, TAKETHIS %q; want 437 and 439.\n"+
					"While it is accepted, the queue line of the full article gets the header-only flag \" H\".\ndqueue/%s: %q",
					c.path, line1, line2, OutLabel, s.queue(t, OutLabel))
			}
		})
	}
}

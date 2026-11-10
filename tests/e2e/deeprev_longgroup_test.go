package e2e

import (
	"strings"
	"testing"

	testkit "github.com/go-while/nntp-testkit"
)

// TestDeepRevLongGroupNameKeepsLaterGroups (L5-08): the reader gives an article a number
// in each group of its Newsgroups: header and writes one overview record per group
// (dreaderd/group.c, NNFeedOverview()). A name longer than a group name can be (256
// characters) is skipped, but the skip left the list cut at that name: the loop ended
// there, and the groups after it got no article number and no overview record, although
// the article was accepted (235).
//
// An article for "e2e.test,<a name of 304 characters>,e2e.other" must show up in e2e.other
// too.
func TestDeepRevLongGroupNameKeepsLaterGroups(t *testing.T) {
	st, spool := spoolStore(t)
	s := NewReaderSite(t, ReaderOptions{Spool: spool})
	id := msgID(t)
	long := "e2e." + strings.Repeat("x", 300)
	feedReader(t, s, st, "e2e.test", newArticle(id, "e2e.test,"+long+",e2e.other", "long group name", "body"))

	c := s.Reader(t)
	defer c.ExpectCmd(205, "QUIT")
	testkit.WaitFor(t, testkit.Deadline(), "the article in GROUP e2e.test", func() bool {
		_, line := c.Cmd("GROUP e2e.test")
		return strings.HasPrefix(line, "211 1 1 1 ")
	})
	if _, line := c.Cmd("GROUP e2e.other"); !strings.HasPrefix(line, "211 1 1 1 ") {
		t.Fatalf("GROUP e2e.other: %q, want 211 1 1 1: the group comes after an over-long name in Newsgroups: and got no article number", line)
	}
	var over []string
	testkit.WaitFor(t, testkit.Deadline(), "the overview record in e2e.other", func() bool {
		if code, _ := c.Cmd("OVER 1"); code != 224 {
			return false
		}
		over = c.ReadDotLines()
		return len(over) == 1
	})
	if !strings.Contains(over[0], id) {
		t.Errorf("OVER 1 in e2e.other: %q, want the record of %s", over, id)
	}
}

package e2e

import (
	"strings"
	"testing"
	"time"

	testkit "github.com/go-while/nntp-testkit"
)

// TestReaderNewsgroupsSpace: dreaderd files a fed article under every group of its Newsgroups:
// list, also with white space around a comma (RFC 5536 3.1.4 allows it) or between two names
// without a comma, which is taken as a separator. dreaderd cut each comma entry at its first space,
// so "a b" was filed under a only and "x a,b" not under a (D-206).
func TestReaderNewsgroupsSpace(t *testing.T) {
	for _, groups := range []string{
		"e2e.test,e2e.other",  // control: no white space
		"e2e.test, e2e.other", // a space after the comma
		"e2e.test ,e2e.other", // a space before the comma
		"e2e.test e2e.other",  // a space and no comma
		"e2e.nogroup e2e.test,e2e.other",
	} {
		t.Run(groups, func(t *testing.T) {
			st, spool := spoolStore(t)
			st.AddGroup("e2e.other", "e2e other group")
			s := NewReaderSite(t, ReaderOptions{Spool: spool})
			id := msgID(t)
			feedReader(t, s, st, "e2e.test", newArticle(id, groups, "groups "+groups, "body"))
			c := s.Reader(t)
			defer c.ExpectCmd(205, "QUIT")
			testkit.WaitFor(t, 10*time.Second, "the article in GROUP e2e.test", func() bool {
				_, line := c.Cmd("GROUP e2e.test")
				return strings.HasPrefix(line, "211 1 ")
			})
			if _, line := c.Cmd("GROUP e2e.other"); !strings.HasPrefix(line, "211 1 ") {
				t.Errorf("Newsgroups: %q: GROUP e2e.other: %q, want the article (211 1 ...)", groups, line)
			}
		})
	}
}

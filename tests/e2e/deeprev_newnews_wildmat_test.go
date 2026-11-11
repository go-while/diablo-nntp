package e2e

import (
	"slices"
	"strings"
	"testing"
	"time"

	testkit "github.com/go-while/nntp-testkit"
)

// TestReaderNewnewsWildmat: the newsgroups argument of NEWNEWS is a wildmat (RFC 3977 4.2,
// 7.4): the comma-separated patterns are tried in order, the last one that matches a group
// decides, and "!" before a pattern excludes the groups it matches. dreaderd matched each
// piece on its own: "!pattern" was taken as a group name, so the groups it should exclude
// were listed, and a group matched by two pieces was listed twice, with each message-id
// printed twice. Now each group is listed once, as the whole wildmat decides.
func TestReaderNewnewsWildmat(t *testing.T) {
	st, spool := spoolStore(t)
	access := strings.Replace(DefaultReaderAccess, "readerdef rp\n", "readerdef rp\n    allownewnews yes\n", 1)
	groups := []string{"comp.lang.c", "comp.lang.go", "comp.os.linux", "misc.test"}
	s := NewReaderSite(t, ReaderOptions{Spool: spool, Access: access, Groups: groups})
	ids := map[string]string{}
	for _, g := range groups {
		id := msgID(t)
		ids[g] = id
		feedReader(t, s, st, g, newArticle(id, g, "wildmat "+g, "body"))
	}

	c := s.Reader(t)
	defer c.ExpectCmd(205, "QUIT")
	for _, g := range groups {
		testkit.WaitFor(t, 10*time.Second, "the article in "+g, func() bool {
			_, line := c.Cmd("GROUP %s", g)
			return strings.HasPrefix(line, "211 1 ")
		})
	}

	for _, tc := range []struct {
		wildmat string
		want    []string // groups whose article is listed, once each
	}{
		{"*", groups},
		{"comp.*,!comp.lang.go", []string{"comp.lang.c", "comp.os.linux"}},
		{"comp.*,comp.lang.c", []string{"comp.lang.c", "comp.lang.go", "comp.os.linux"}},
		{"comp.lang.c,comp.lang.c", []string{"comp.lang.c"}},
		{"comp.lang.*,!comp.lang.*,comp.lang.go", []string{"comp.lang.go"}},
		{"*,!comp.*", []string{"misc.test"}},
		{"*,!comp.lang.?o,!misc.*", []string{"comp.lang.c", "comp.os.linux"}},
		{"!comp.*", nil},
	} {
		c.ExpectCmd(230, "NEWNEWS %s 000101 000000 GMT", tc.wildmat)
		got := c.ReadDotLines()
		var want []string
		for _, g := range tc.want {
			want = append(want, ids[g])
		}
		slices.Sort(want)
		sorted := slices.Clone(got)
		slices.Sort(sorted)
		if !slices.Equal(sorted, want) {
			t.Errorf("NEWNEWS %s: %q, want each of %q once (groups %q)", tc.wildmat, got, want, tc.want)
		}
	}
}

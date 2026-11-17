package e2e

// dreaderd: an article that names one group more than once in Newsgroups gets one article
// number and one overview record in that group (D-226). The feed numbered every name of the
// list: each repeat took a number and wrote a record, so 512 or more repeats (the default
// index size) overwrote the index slots of the older articles of the group, which then
// answered 423 and dropped out of OVER.

import (
	"strconv"
	"strings"
	"testing"

	testkit "github.com/go-while/nntp-testkit"
)

// overXref returns the groups and numbers of the Xref field of an OVER line, as
// "group:number" strings.
func overXref(t *testing.T, line string) []string {
	t.Helper()
	for _, f := range strings.Split(line, "\t") {
		if v, ok := strings.CutPrefix(f, "Xref: "); ok {
			// the first word is the Xref host
			if w := strings.Fields(v); len(w) > 1 {
				return w[1:]
			}
			return nil
		}
	}
	t.Errorf("no Xref field in OVER line %q", line)
	return nil
}

// TestReaderRepeatedGroupOneNumber: an article with "e2e.test,e2e.other,e2e.test", ten
// normal articles, one article with e2e.test 600 times and e2e.other a few times, and one
// more. Each article takes one number in each of its groups, the older articles keep their
// overview records, the Xref names each group once, and the stored Newsgroups header is the
// one the peer sent.
func TestReaderRepeatedGroupOneNumber(t *testing.T) {
	st, spool := spoolStore(t)
	s := NewReaderSite(t, ReaderOptions{Spool: spool})

	var ids []string
	add := func(newsgroups, subject string) {
		id := msgID(t)
		ids = append(ids, id)
		feedReader(t, s, st, "e2e.test", newArticle(id, newsgroups, subject, "body"))
	}
	add("e2e.test,e2e.other,e2e.test", "a group twice") // e2e.test 1, e2e.other 1
	for i := range 10 {
		add("e2e.test", "normal "+strconv.Itoa(i)) // e2e.test 2..11
	}
	var b strings.Builder
	b.WriteString("e2e.test,e2e.other")
	for i := range 599 {
		b.WriteString(",e2e.test")
		if i%100 == 0 {
			b.WriteString(",e2e.other")
		}
	}
	rep := b.String()
	add(rep, "a group 600 times") // e2e.test 12, e2e.other 2
	add("e2e.test", "after")      // e2e.test 13

	c := s.Reader(t)
	group := func(name string) (count, low, high int) {
		_, line := c.Cmd("GROUP %s", name)
		f := strings.Fields(line)
		if len(f) < 4 || f[0] != "211" {
			t.Fatalf("GROUP %s: %q, want 211", name, line)
		}
		count, _ = strconv.Atoi(f[1])
		low, _ = strconv.Atoi(f[2])
		high, _ = strconv.Atoi(f[3])
		return
	}
	// the overview may reach the reader fork a little after the 235
	testkit.WaitFor(t, testkit.Deadline(), "the high mark of e2e.test at 13 or above", func() bool {
		_, _, h := group("e2e.test")
		return h >= 13
	})

	if n, l, h := group("e2e.other"); n != 2 || l != 1 || h != 2 {
		t.Errorf("GROUP e2e.other: count %d low %d high %d, want 2 1 2: a repeated name took more than one number", n, l, h)
	}
	if n, l, h := group("e2e.test"); n != 13 || l != 1 || h != 13 {
		t.Errorf("GROUP e2e.test: count %d low %d high %d, want 13 1 13: a repeated name took more than one number", n, l, h)
	}

	// every article is still there under its own number
	for i, id := range ids {
		if code, line := c.Cmd("STAT %d", i+1); code != 223 || !strings.Contains(line, id) {
			t.Errorf("STAT %d: %q, want 223 with %s", i+1, line, id)
		}
	}
	code, line := c.Cmd("OVER 1-13")
	if code != 224 {
		t.Fatalf("OVER 1-13: %q, want 224", line)
	}
	over := c.ReadDotLines()
	if len(over) != 13 {
		t.Errorf("OVER 1-13: %d lines, want 13: %q", len(over), over)
	}
	for i, l := range over {
		if i >= len(ids) || !strings.HasPrefix(l, strconv.Itoa(i+1)+"\t") || !strings.Contains(l, ids[i]) {
			t.Errorf("OVER line %d: %.200q, want article %d %s", i+1, l, i+1, ids[min(i, len(ids)-1)])
		}
	}

	// the Xref of the two articles with repeats: each group once, with its one number
	check := func(n int, want ...string) {
		t.Helper()
		if n > len(over) {
			return
		}
		got := overXref(t, over[n-1])
		if len(got) != len(want) {
			t.Errorf("Xref of article %d: %q, want each of %q once", n, got, want)
			return
		}
		for _, w := range want {
			found := 0
			for _, g := range got {
				if g == w {
					found++
				}
			}
			if found != 1 {
				t.Errorf("Xref of article %d: %q, want %s once", n, got, w)
			}
		}
	}
	check(1, "e2e.test:1", "e2e.other:1")
	check(12, "e2e.test:12", "e2e.other:2")

	// e2e.other: the same two articles, numbers 1 and 2
	c.ExpectCmd(211, "GROUP e2e.other")
	c.ExpectCmd(224, "OVER 1-2")
	if got := c.ReadDotLines(); len(got) != 2 || !strings.Contains(got[0], ids[0]) || !strings.Contains(got[1], ids[11]) {
		t.Errorf("OVER 1-2 in e2e.other: %.300q, want %s and %s", got, ids[0], ids[11])
	}

	// the stored header is the one the peer sent: the list is not rewritten
	c.ExpectCmd(211, "GROUP e2e.test")
	c.ExpectCmd(221, "HEAD 12")
	if got := headerValue(c.ReadDotLines(), "Newsgroups"); got != rep {
		t.Errorf("HEAD 12: Newsgroups of %d bytes, want the %d bytes the peer sent", len(got), len(rep))
	}
	c.ExpectCmd(205, "QUIT")
}

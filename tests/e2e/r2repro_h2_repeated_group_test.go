package e2e

// Deep review round 2, H2 (reproduction): an article that names one group several times in
// Newsgroups gets one article number and one overview record per repeat (dreaderd/group.c).

import (
	"strings"
	"testing"
	"time"

	testkit "github.com/go-while/nntp-testkit"
)

// TestR2ReproRepeatedGroupOverview: ten articles, then one that lists e2e.test 600 times, then
// one more. The group moves by one number per article, and the ten older articles are still in
// the overview.
func TestR2ReproRepeatedGroupOverview(t *testing.T) {
	st, spool := spoolStore(t)
	s := NewReaderSite(t, ReaderOptions{Spool: spool})
	var ids []string
	for i := range 10 {
		id := msgID(t)
		ids = append(ids, id)
		feedReader(t, s, st, "e2e.test", newArticle(id, "e2e.test", "normal "+string(rune('a'+i)), "body"))
	}
	c := s.Reader(t)
	group := func() (count, low, high string) {
		_, line := c.Cmd("GROUP e2e.test")
		f := strings.Fields(line)
		if len(f) < 4 || f[0] != "211" {
			t.Fatalf("GROUP e2e.test: %q", line)
		}
		return f[1], f[2], f[3]
	}
	testkit.WaitFor(t, testkit.Deadline(), "the ten articles in GROUP e2e.test", func() bool {
		n, _, _ := group()
		return n == "10"
	})
	n0, l0, h0 := group()
	t.Logf("before the repeated article: count %s low %s high %s", n0, l0, h0)

	rep := strings.TrimSuffix(strings.Repeat("e2e.test,", 600), ",")
	feedReader(t, s, st, "e2e.test", newArticle(msgID(t), rep, "repeated", "body"))
	feedReader(t, s, st, "e2e.test", newArticle(msgID(t), "e2e.test", "after", "body"))
	// the feed's overview write may come after the 235: wait for the high mark to pass the
	// ten, then let any further writes of the same article finish
	testkit.WaitFor(t, testkit.Deadline(), "the high mark of e2e.test above 11", func() bool {
		_, _, h := group()
		return h != h0 && h != "11"
	})
	time.Sleep(500 * time.Millisecond)
	n1, l1, h1 := group()
	t.Logf("after the repeated article and one more: count %s low %s high %s", n1, l1, h1)

	// 10 older + the repeated one + the next one: numbers 1..12
	if h1 != "12" || n1 != "12" {
		t.Errorf("GROUP e2e.test: count %s low %s high %s, want count 12 low 1 high 12: the repeated article took more than one number", n1, l1, h1)
	}
	code, line := c.Cmd("OVER 1-10")
	if code != 224 {
		t.Errorf("OVER 1-10: %q, want 224 with the ten older articles", line)
	} else if got := c.ReadDotLines(); len(got) != 10 {
		t.Errorf("OVER 1-10: %d lines, want 10: %q", len(got), got)
	}
	for i := 1; i <= 10; i++ {
		if code, line := c.Cmd("STAT %d", i); code != 223 {
			t.Errorf("STAT %d (%s): %q, want 223", i, ids[i-1], line)
		}
	}
}

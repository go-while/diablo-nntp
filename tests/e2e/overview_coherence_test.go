package e2e

import (
	"strings"
	"testing"

	testkit "github.com/go-while/nntp-testkit"
)

// Keep the reader and its overview window open while the feeder appends.
func TestOverviewRefreshAfterFeed(t *testing.T) {
	st, spool := spoolStore(t)
	s := NewReaderSite(t, ReaderOptions{Spool: spool})
	first, second := msgID(t), msgID(t)
	feedReader(t, s, st, "e2e.test", newArticle(first, "e2e.test", "first", "body"))
	c := s.Reader(t)
	expect(t, c, 211, "GROUP e2e.test")
	expect(t, c, 224, "OVER 1")
	if lines := c.ReadDotLines(); len(lines) != 1 || !strings.Contains(lines[0], first) {
		t.Fatalf("initial OVER: %q", lines)
	}
	feedReader(t, s, st, "e2e.test", newArticle(second, "e2e.test", "second", "body"))
	waitFor(t, testkit.Deadline(), "second article in cached overview", func() bool {
		code, _ := c.Cmd("GROUP e2e.test")
		if code != 211 {
			return false
		}
		code, _ = c.Cmd("OVER 1-2")
		if code != 224 {
			return false
		}
		lines := c.ReadDotLines()
		return len(lines) == 2 && strings.Contains(lines[0], first) && strings.Contains(lines[1], second)
	})
	expect(t, c, 205, "QUIT")
}

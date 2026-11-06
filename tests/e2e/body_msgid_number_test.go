package e2e

import (
	"strings"
	"testing"
)

// TestReaderBodyByMessageIdNumber (D-196): BODY by message-id answers "222 0 <id>", as
// ARTICLE, HEAD and STAT do (RFC 3977 6.2.3), also with a group selected and the body coming
// from the spool server; BODY by number still answers that number. Before, the spool path
// wrote the current article's number ("222 1 <id>" after GROUP).
func TestReaderBodyByMessageIdNumber(t *testing.T) {
	st, spool := spoolStore(t)
	s := NewReaderSite(t, ReaderOptions{Spool: spool})
	first, other := msgID(t), msgID(t)
	feedReader(t, s, st, "e2e.test", newArticle(first, "e2e.test", "first", "first body"))
	feedReader(t, s, st, "e2e.other", newArticle(other, "e2e.other", "other", "other body"))
	c := s.Reader(t)
	defer c.ExpectCmd(205, "QUIT")
	c.ExpectCmd(211, "GROUP e2e.test") // current article 1
	for _, id := range []string{other, first} {
		code, line := c.Cmd("BODY %s", id)
		if code != 222 {
			t.Fatalf("BODY %s: %q, want 222", id, line)
		}
		if want := "222 0 " + id; !strings.HasPrefix(line, want) {
			t.Errorf("BODY %s: %q, want %q...", id, line, want)
		}
		c.ReadDotLines()
	}
	code, line := c.Cmd("BODY 1")
	if code != 222 || !strings.HasPrefix(line, "222 1 "+first) {
		t.Errorf("BODY 1: %q, want \"222 1 %s ...\"", line, first)
	}
	c.ReadDotLines()
}

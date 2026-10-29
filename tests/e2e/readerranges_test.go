package e2e

import (
	"strings"
	"testing"
)

// TestReaderRangeBeyondLimit (D-147, D-148): an article number above the RFC 3977 limit
// (2147483647, XNUMBERING rfc3977) in an OVER, HDR or LISTGROUP range is a 501, not a bound. The failed
// conversion (-1) was used like a number: "OVER 4294967297-" was clamped to the group's low
// mark and returned the whole group, and an oversized upper bound ended the range below its
// start.
func TestReaderRangeBeyondLimit(t *testing.T) {
	st, spool := spoolStore(t)
	s := NewReaderSite(t, ReaderOptions{Spool: spool})
	id := msgID(t)
	feedReader(t, s, st, "e2e.test", newArticle(id, "e2e.test", "range limit", "body"))

	c := s.Reader(t)
	defer expect(t, c, 205, "QUIT")
	expect(t, c, 211, "GROUP e2e.test")
	expect(t, c, 203, "XNUMBERING rfc3977") // the limit applies to RFC 3977 numbering only

	for _, rng := range []string{"4294967297-", "1-4294967297", "4294967297", "4294967297-4294967298"} {
		expect(t, c, 501, "OVER %s", rng)
		expect(t, c, 501, "HDR Subject %s", rng)
		expect(t, c, 501, "LISTGROUP e2e.test %s", rng)
	}
	// ordinary ranges still work
	expect(t, c, 224, "OVER 1-")
	if got := c.ReadDotLines(); len(got) != 1 {
		t.Errorf("OVER 1-: %q, want one line", got)
	}
	expect(t, c, 211, "LISTGROUP e2e.test 1-1")
	if got := c.ReadDotLines(); len(got) != 1 || got[0] != "1" {
		t.Errorf("LISTGROUP e2e.test 1-1: %q, want [1]", got)
	}
}

// TestReaderNewnewsKeepsSelectedGroup (D-149): NEWNEWS walks the matching groups through the
// connection's current-group slot; the group the client selected before must still be the
// current one afterwards. STAT 1 after NEWNEWS was 412 "no newsgroup selected".
func TestReaderNewnewsKeepsSelectedGroup(t *testing.T) {
	st, spool := spoolStore(t)
	access := strings.Replace(DefaultReaderAccess, "readerdef rp\n", "readerdef rp\n    allownewnews yes\n", 1)
	s := NewReaderSite(t, ReaderOptions{Spool: spool, Access: access})
	id := msgID(t)
	feedReader(t, s, st, "e2e.test", newArticle(id, "e2e.test", "newnews", "body"))

	c := s.Reader(t)
	defer expect(t, c, 205, "QUIT")
	expect(t, c, 211, "GROUP e2e.test")
	expect(t, c, 230, "NEWNEWS * 700101 000000 GMT")
	c.ReadDotLines() // the list itself is not what is tested
	expect(t, c, 223, "STAT 1")
}

package e2e

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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
	// a refused LISTGROUP does not change the selected group
	expect(t, c, 211, "GROUP e2e.test")
	expect(t, c, 501, "LISTGROUP e2e.other 4294967297-")
	expect(t, c, 223, "STAT 1")
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
// connection's current-group slot; the group and article the client selected before must be
// the current ones afterwards, whatever the list: all existing groups, a group that does not
// exist first, in the middle or alone (the scan skips it and goes on). STAT 1 after NEWNEWS
// was 412 "no newsgroup selected", and a missing group ended the command early.
func TestReaderNewnewsKeepsSelectedGroup(t *testing.T) {
	st, spool := spoolStore(t)
	access := strings.Replace(DefaultReaderAccess, "readerdef rp\n", "readerdef rp\n    allownewnews yes\n", 1)
	s := NewReaderSite(t, ReaderOptions{Spool: spool, Access: access})
	id1, id2 := msgID(t), msgID(t)
	feedReader(t, s, st, "e2e.test", newArticle(id1, "e2e.test", "newnews one", "body"), newArticle(id2, "e2e.test", "newnews two", "body"))

	c := s.Reader(t)
	defer expect(t, c, 205, "QUIT")
	waitFor(t, 10*time.Second, "two articles in GROUP e2e.test", func() bool {
		_, line := c.Cmd("GROUP e2e.test")
		return strings.HasPrefix(line, "211 2")
	})
	for _, groups := range []string{"*", "e2e.test", "no.such.group", "no.such.group,e2e.test", "e2e.test,no.such.group,e2e.other"} {
		expect(t, c, 211, "GROUP e2e.test")
		expect(t, c, 223, "STAT 2") // the current article is 2
		expect(t, c, 230, "NEWNEWS %s 000101 000000 GMT", groups)
		got := c.ReadDotLines()
		if strings.Contains(groups, "e2e.test") && len(got) != 2 {
			t.Errorf("NEWNEWS %s: %q, want both articles (a missing group must not end the list)", groups, got)
		}
		line := expect(t, c, 223, "STAT") // the same group and article as before
		if f := strings.Fields(line); len(f) < 2 || f[1] != "2" {
			t.Errorf("after NEWNEWS %s: STAT answers %q, want article 2 of the selected group", groups, line)
		}
	}
}

// TestReaderNewnewsUnreadableOverview (D-149): a group whose overview index cannot be opened is
// skipped by the NEWNEWS scan like a missing group: the groups after it are listed and the
// client's group stays selected. It ended the list and left the scan's group selected.
func TestReaderNewnewsUnreadableOverview(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root can read any file")
	}
	st, spool := spoolStore(t)
	st.AddGroup("e2e.other", "second group")
	access := strings.Replace(DefaultReaderAccess, "readerdef rp\n", "readerdef rp\n    allownewnews yes\n", 1)
	s := NewReaderSite(t, ReaderOptions{Spool: spool, Access: access})
	id1, id2 := msgID(t), msgID(t)
	feedReader(t, s, st, "e2e.test", newArticle(id1, "e2e.test", "first group", "body"))
	feedReader(t, s, st, "e2e.other", newArticle(id2, "e2e.other", "second group", "body"))

	// the overview index of e2e.other, from the group name in its header
	var other string
	waitFor(t, 10*time.Second, "both overview indexes", func() bool {
		n := 0
		filepath.WalkDir(filepath.Join(s.Dir, "spool", "group"), func(p string, d os.DirEntry, err error) error {
			if err == nil && !d.IsDir() && strings.HasPrefix(d.Name(), "over.") {
				n++
				if b, err := os.ReadFile(p); err == nil && bytes.Contains(b[:min(len(b), 300)], []byte("e2e.other")) {
					other = p
				}
			}
			return nil
		})
		return n == 2 && other != ""
	})
	if err := os.Chmod(other, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(other, 0o644) })

	c := s.Reader(t)
	defer expect(t, c, 205, "QUIT")
	expect(t, c, 211, "GROUP e2e.test")
	expect(t, c, 223, "STAT 1")
	expect(t, c, 230, "NEWNEWS e2e.other,e2e.test 000101 000000 GMT")
	if got := c.ReadDotLines(); len(got) != 1 || got[0] != id1 {
		t.Errorf("NEWNEWS with an unreadable overview: %q, want [%s] (the group after it)", got, id1)
	}
	expect(t, c, 223, "STAT")
}

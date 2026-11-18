package e2e

// Articles the external spam filter (feederfilter) must get although diab_read() failed on
// them before (D-241). Helpers: filter_firstline_test.go.

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

// TestFilterUnreadArticlesJudged (D-241): diab_read() failed on any NUL
// byte in the stored article and on the records of a compressed spool object (compresslvl);
// DiabFilter() then sent nothing and the article was not judged, so it reached the
// nospam feed unjudged (and the onlyspam feed too). perDotFilter calls the articles with the
// subject "spam" spam and the others not. Each article must get its own verdict: a spam
// article with a NUL byte goes only to the onlyspam feed, a plain one after it only to the
// nospam feed (the NUL does not put the filter out of step); on a compressed spool the same
// for a small spam article and a large plain one (more than one read of the gzip member).
// Fails on main: the NUL article and the articles on the compressed spool are in both queues.
func TestFilterUnreadArticlesJudged(t *testing.T) {
	check := func(t *testing.T, s *Site, ids []string, wantHam, wantSpam []bool) {
		t.Helper()
		var ham, spam []bool
		for limit := time.Now().Add(15 * time.Second); ; time.Sleep(200 * time.Millisecond) {
			ham, spam = queued(t, s, OutLabel, ids...), queued(t, s, "spamonly", ids...)
			if (slices.Equal(ham, wantHam) && slices.Equal(spam, wantSpam)) || time.Now().After(limit) {
				break
			}
		}
		if !slices.Equal(ham, wantHam) {
			t.Errorf("queued for the nospam feed: %v, want %v", ham, wantHam)
		}
		if !slices.Equal(spam, wantSpam) {
			t.Errorf("queued for the onlyspam feed: %v, want %v", spam, wantSpam)
		}
		b, _ := os.ReadFile(filepath.Join(s.Dir, "filter.log"))
		if n := strings.Count(string(b), "\n.\r\n"); n != len(ids) {
			t.Errorf("the filter got %d articles, want %d", n, len(ids))
		}
	}

	t.Run("nul", func(t *testing.T) {
		s := filterSite(t, perDotFilter, "", nil)
		a, b, c := msgID(t), msgID(t), msgID(t)
		injectLines(t, s, a, newArticle(a, "e2e.test", "spam", "body"))
		injectLines(t, s, b, newArticle(b, "e2e.test", "spam", "a\x00b", "after the NUL"))
		injectLines(t, s, c, newArticle(c, "e2e.test", "ham", "body"))
		check(t, s, []string{a, b, c}, []bool{false, false, true}, []bool{true, true, false})
	})

	t.Run("compressed", func(t *testing.T) {
		skipWithoutZlib(t)
		s := filterSite(t, perDotFilter, compressedSpoolCtl, nil)
		a, b := msgID(t), msgID(t)
		var big []string
		for i := range 4000 {
			big = append(big, fmt.Sprintf("line %04d of a long article body, compressed on the spool", i))
		}
		injectLines(t, s, a, newArticle(a, "e2e.test", "spam", "body"))
		injectLines(t, s, b, newArticle(b, "e2e.test", "ham", big...))
		check(t, s, []string{a, b}, []bool{false, true}, []bool{true, false})
		log, _ := os.ReadFile(filepath.Join(s.Dir, "filter.log"))
		if !strings.Contains(string(log), "line 3999 of a long article body") {
			t.Errorf("the filter did not get the whole large article")
		}
	})
}

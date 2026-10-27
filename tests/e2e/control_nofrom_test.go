package e2e

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

// TestControlWithoutFromNoFdLeak: a fed article with a Control: header but no From: makes
// ParseControl() return early. It opened dcontrol.ctl before checking From: and did not close
// it, so every such article cost the reader feed process one descriptor until it ran out
// (GHSA-qjg5-xp53-hj7h). After 40 of them the dreaderd processes of the site must not hold
// (nearly) 40 more descriptors. Linux only (/proc).
func TestControlWithoutFromNoFdLeak(t *testing.T) {
	st, spool := spoolStore(t)
	s := NewReaderSite(t, ReaderOptions{
		Spool: spool,
		Files: map[string]string{"dcontrol.ctl": "newgroup:*:*:doit\n"},
	})
	noFrom := func(i int) []string {
		var a []string
		for _, l := range newArticle(msgID(t), "e2e.test", fmt.Sprintf("cmsg cancel %d", i), "body") {
			if strings.HasPrefix(l, "From: ") {
				continue
			}
			a = append(a, l)
		}
		// the Control: header goes into the head, before the blank line
		for j, l := range a {
			if l == "" {
				return append(append(append([]string{}, a[:j]...), fmt.Sprintf("Control: cancel <gone%d@e2e.test>", i)), a[j:]...)
			}
		}
		t.Fatal("no blank line in the article")
		return nil
	}
	fds := func() int {
		pids := sitePids(s, "dreaderd")
		if len(pids) == 0 {
			t.Skip("no /proc or no dreaderd processes found")
		}
		n := 0
		for _, pid := range pids {
			if c := fdCount(pid); c > 0 {
				n += c
			}
		}
		return n
	}

	// One first, so the feed process exists and has opened what it keeps open anyway.
	feedReader(t, s, st, "e2e.test", noFrom(0))
	time.Sleep(500 * time.Millisecond)
	before := fds()
	var arts [][]string
	for i := 1; i <= 40; i++ {
		arts = append(arts, noFrom(i))
	}
	feedReader(t, s, st, "e2e.test", arts...)
	time.Sleep(time.Second)
	after := fds()
	if after-before >= 20 {
		t.Errorf("40 control articles without From: left the dreaderd processes with %d more open descriptors (%d -> %d)", after-before, before, after)
	}
}

package e2e

// D-235 (deep review round 2, M7): dexpireover -R on a group written with
// "readerxover trackonly" (util/dexpireover.c rewriteDataFile).

import (
	"bytes"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	testkit "github.com/go-while/nntp-testkit"
)

// trackonlyConfig is the diablo.config of a trackonly reader: on a build with Cancel-Lock
// support dreaderd refuses trackonly with the default readercanlock, a build without it does
// not know the directive.
func trackonlyConfig(tb testing.TB) map[string]string {
	tb.Helper()
	cfg := map[string]string{"readerxover": "trackonly"}
	if b, err := os.ReadFile(filepath.Join(bin(tb), "dreaderd")); err == nil && bytes.Contains(b, []byte("readercanlock")) {
		cfg["readercanlock"] = "none"
	}
	return cfg
}

// TestDexpireoverRewriteTrackonly: with readerxover trackonly the index of a group holds the
// article numbers and no overview data (every record has no data offset). "dexpireover -a -R",
// which the shipped weekly.reader runs, must keep those records: the index still holds
// article 1, the group its twenty articles, the next "dexpireover -a" leaves its range alone,
// and a new article gets the next number. Fails before the fix: -R wrote an empty index, and the next run moved the
// low number past all twenty articles (GROUP 211 1 20 20).
func TestDexpireoverRewriteTrackonly(t *testing.T) {
	st, spool := spoolStore(t)
	s := NewReaderSite(t, ReaderOptions{Spool: spool, Config: trackonlyConfig(t)})
	for i := range 20 {
		feedReader(t, s, st, "e2e.test", newArticle(msgID(t), "e2e.test", "trackonly "+strconv.Itoa(i), "body"))
	}
	group := func() string {
		c := s.Reader(t)
		defer c.Close()
		_, line := c.Cmd("GROUP e2e.test")
		return line
	}
	// groupinfo reads the index slot of the group's low number: "StartNo=1" while article 1
	// is still listed there. (LISTGROUP lists no article of a trackonly group at all.)
	slot1 := func(when string) {
		t.Helper()
		info, err := s.TryRun("dgrpctl", "groupinfo", "e2e.test")
		if err != nil || !strings.Contains(info, "StartNo=1\n") || strings.Contains(info, "artNoMismatch") {
			t.Errorf("dgrpctl groupinfo e2e.test %s: err %v, the index no longer has article 1:\n%s", when, err, info)
		}
	}
	testkit.WaitFor(t, testkit.Deadline(), "20 articles in GROUP e2e.test", func() bool {
		return strings.HasPrefix(group(), "211 20 1 20 ")
	})
	g0 := group()
	slot1("before dexpireover")
	// dreaderd's feed process keeps a shared lock on the index of a group it wrote to until
	// its next flush (every 5 minutes in a feed-only process); dexpireover -R needs the
	// exclusive lock and finds the group "in use" until then. Stop dreaderd for the runs.
	restart := func() {
		startReader(t, s, "[127.0.0.1]:"+strings.TrimPrefix(s.Addr, "127.0.0.1:"))
	}
	s.Proc.Stop()
	out, err := s.TryRun("dexpireover", "-a", "-R", "-v")
	t.Logf("dexpireover -a -R -v: err %v\n%s", err, out)
	if err != nil {
		t.Errorf("dexpireover -a -R: %v", err)
	}
	if strings.Contains(out, "records are lost") || strings.Contains(out, "failed") {
		t.Errorf("dexpireover -a -R reports a failure for a trackonly group:\n%s", out)
	}
	slot1("after dexpireover -a -R")
	restart()
	if g1 := group(); !strings.HasPrefix(g1, "211 20 1 20 ") {
		t.Errorf("GROUP e2e.test after dexpireover -a -R: %q, want 211 20 1 20 (before: %q)", g1, g0)
	}

	// The next ordinary run must not take the group for one without articles.
	s.Proc.Stop()
	out, err = s.TryRun("dexpireover", "-a", "-v")
	t.Logf("dexpireover -a -v: err %v\n%s", err, out)
	if err != nil {
		t.Errorf("dexpireover -a: %v", err)
	}
	restart()
	if g2 := group(); !strings.HasPrefix(g2, "211 20 1 20 ") {
		t.Errorf("GROUP e2e.test after the next dexpireover -a: %q, want 211 20 1 20", g2)
	}

	// A new article goes on from 21, and a second -R keeps all of them.
	feedReader(t, s, st, "e2e.test", newArticle(msgID(t), "e2e.test", "trackonly 20", "body"))
	testkit.WaitFor(t, testkit.Deadline(), "21 articles in GROUP e2e.test", func() bool {
		return strings.HasPrefix(group(), "211 21 1 21 ")
	})
	s.Proc.Stop()
	out, err = s.TryRun("dexpireover", "-a", "-R", "-v")
	t.Logf("second dexpireover -a -R -v: err %v\n%s", err, out)
	if err != nil {
		t.Errorf("second dexpireover -a -R: %v", err)
	}
	slot1("after the second dexpireover -a -R")
	out, err = s.TryRun("dexpireover", "-a", "-v")
	t.Logf("dexpireover -a -v after it: err %v\n%s", err, out)
	restart()
	if g3 := group(); !strings.HasPrefix(g3, "211 21 1 21 ") {
		t.Errorf("GROUP e2e.test after the second dexpireover -a -R and -a: %q, want 211 21 1 21", g3)
	}
}

package e2e

// Deep review round 2, M7 and M8 (reproduction): dexpireover -R on a trackonly group, and on a
// full group the feed is writing into (util/dexpireover.c rewriteDataFile / RewriteData).

import (
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	testkit "github.com/go-while/nntp-testkit"
)

// TestR2ReproTrackonlyRewrite: readerxover trackonly stores no overview data, only the index of
// article numbers. "dexpireover -a -R" must leave the twenty articles listed.
func TestR2ReproTrackonlyRewrite(t *testing.T) {
	skipWithoutCanLockMode(t)
	st, spool := spoolStore(t)
	s := NewReaderSite(t, ReaderOptions{Spool: spool,
		Config: map[string]string{"readerxover": "trackonly", "readercanlock": "none"}})
	for i := range 20 {
		feedReader(t, s, st, "e2e.test", newArticle(msgID(t), "e2e.test", "trackonly "+strconv.Itoa(i), "body"))
	}
	group := func() string {
		c := s.Reader(t)
		defer c.Close()
		_, line := c.Cmd("GROUP e2e.test")
		return line
	}
	testkit.WaitFor(t, testkit.Deadline(), "20 articles in GROUP e2e.test", func() bool {
		return strings.HasPrefix(group(), "211 20 1 20")
	})
	g0 := group()
	info0, _ := s.TryRun("dgrpctl", "groupinfo", "e2e.test")
	out, err := s.TryRun("dexpireover", "-a", "-R", "-v")
	t.Logf("dexpireover -a -R -v: err %v\n%s", err, out)
	g1 := group()
	info1, _ := s.TryRun("dgrpctl", "groupinfo", "e2e.test")
	t.Logf("before: %q\n%s\nafter: %q\n%s", g0, info0, g1, info1)
	if !strings.HasPrefix(g1, "211 20 1 20") {
		t.Errorf("GROUP e2e.test after dexpireover -a -R: %q, want 211 20 1 20 (before: %q)", g1, g0)
	}
}

// TestR2ReproRewriteFullGroupBeingWritten: a group whose index is full (512 slots) while the
// feed keeps adding articles; dexpireover -a -R is run again and again. Every run must exit 0.
func TestR2ReproRewriteFullGroupBeingWritten(t *testing.T) {
	_, spool := spoolStore(t)
	s := NewReaderSite(t, ReaderOptions{Spool: spool})
	p := s.dialFrom(t, PeerIP)
	article := func(i int) (string, []byte) {
		id := msgID(t)
		a := newArticle(id, "e2e.test", "full "+strconv.Itoa(i), "body")
		return id, []byte(strings.Join(a, "\r\n") + "\r\n")
	}
	for i := range 560 { // past the 512 slots
		id, a := article(i)
		p.ExpectIhave(235, id, a)
	}
	var wg sync.WaitGroup
	done := make(chan struct{})
	var mu sync.Mutex
	fed := 0
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer close(done)
		for i := 560; i < 4000; i++ {
			id, a := article(i)
			code, _, _, err := p.TryIhave(id, a)
			if err != nil || code != 235 {
				return
			}
			mu.Lock()
			fed++
			mu.Unlock()
		}
	}()
	runs, bad := 0, 0
	deadline := time.Now().Add(120 * time.Second)
loop:
	for time.Now().Before(deadline) {
		out, err := s.TryRun("dexpireover", "-a", "-R")
		runs++
		if err != nil || strings.Contains(out, "panic") {
			bad++
			t.Errorf("run %d: err %v\n%s", runs, err, out)
		}
		select {
		case <-done:
			if runs >= 20 {
				break loop
			}
		default:
		}
	}
	<-done
	wg.Wait()
	mu.Lock()
	t.Logf("%d runs of dexpireover -a -R while the feed added %d articles: %d failed", runs, fed, bad)
	mu.Unlock()
}

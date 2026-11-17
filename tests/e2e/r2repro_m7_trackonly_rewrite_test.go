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
	// dreaderd's feed process keeps a shared lock on the index of a group it wrote to (it lets go
	// at the next flush, every 5 minutes in a feed-only fork, reader.c): dexpireover -R, which
	// needs the exclusive lock, finds the group "in use" until then. Stop dreaderd first.
	s.Proc.Stop()
	out, err := s.TryRun("dexpireover", "-a", "-R", "-v")
	t.Logf("dexpireover -a -R -v: err %v\n%s", err, out)
	startReader(t, s, "[127.0.0.1]:"+strings.TrimPrefix(s.Addr, "127.0.0.1:"))
	g1 := group()
	info1, _ := s.TryRun("dgrpctl", "groupinfo", "e2e.test")
	t.Logf("before: %q\n%s\nafter: %q\n%s", g0, info0, g1, info1)
	if !strings.HasPrefix(g1, "211 20 1 20") {
		t.Errorf("GROUP e2e.test after dexpireover -a -R: %q, want 211 20 1 20 (before: %q)", g1, g0)
	}
	if strings.Contains(info1, "artNoMismatch") || strings.Contains(info1, "Article not found") {
		t.Errorf("dgrpctl groupinfo after dexpireover -a -R: the index no longer has article 1 (the entries were dropped):\n%s", info1)
	}
	// the next ordinary expire run takes the empty index for a group without articles
	s.Proc.Stop()
	out2, err2 := s.TryRun("dexpireover", "-a", "-v")
	startReader(t, s, "[127.0.0.1]:"+strings.TrimPrefix(s.Addr, "127.0.0.1:"))
	g2 := group()
	t.Logf("second run dexpireover -a -v: err %v\n%s\nGROUP after it: %q", err2, out2, g2)
	if !strings.HasPrefix(g2, "211 20 1 20") {
		t.Errorf("GROUP e2e.test after the next dexpireover -a: %q, want 211 20 1 20", g2)
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
	deadline := time.Now().Add(90 * time.Second)
	locked := 0
loop:
	for time.Now().Before(deadline) {
		out, err := s.TryRun("dexpireover", "-a", "-R", "-l3", "-v3")
		runs++
		if !strings.Contains(out, "file in use") {
			locked++
		}
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
	t.Logf("%d runs of dexpireover -a -R while the feed added %d articles: %d failed, %d runs without a \"file in use\" message", runs, fed, bad, locked)
	mu.Unlock()
}

// TestR2ReproRewriteAfterRepeatedGroup: the state H2 leaves behind (NB..NE wider than the 512
// index slots) and dexpireover -a -R with dreaderd stopped (so that the lock is free).
func TestR2ReproRewriteAfterRepeatedGroup(t *testing.T) {
	st, spool := spoolStore(t)
	s := NewReaderSite(t, ReaderOptions{Spool: spool})
	for i := range 10 {
		feedReader(t, s, st, "e2e.test", newArticle(msgID(t), "e2e.test", "normal "+strconv.Itoa(i), "body"))
	}
	rep := strings.TrimSuffix(strings.Repeat("e2e.test,", 600), ",")
	feedReader(t, s, st, "e2e.test", newArticle(msgID(t), rep, "repeated", "body"))
	c := s.Reader(t)
	testkit.WaitFor(t, testkit.Deadline(), "the repeated article numbered", func() bool {
		_, line := c.Cmd("GROUP e2e.test")
		return strings.Fields(line)[len(strings.Fields(line))-2] != "10"
	})
	time.Sleep(time.Second)
	_, g0 := c.Cmd("GROUP e2e.test")
	s.Proc.Stop()
	out, err := s.TryRun("dexpireover", "-a", "-R", "-v")
	t.Logf("group before: %q\ndexpireover -a -R -v: err %v\n%s", g0, err, out)
	if err != nil || strings.Contains(out, "panic") {
		t.Errorf("dexpireover -a -R on a group whose range is wider than its index: err %v", err)
	}
}

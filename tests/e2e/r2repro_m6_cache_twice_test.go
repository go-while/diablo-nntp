package e2e

// Deep review round 2, M6 (reproduction): two fetches of one uncached article in one reader process
// share the cache's .tmp file (the fcntl lock belongs to the process, dreaderd/cache.c CreateCache).

import (
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-while/nntp-testkit/fakeserver"
)

// TestR2ReproCacheSameArticleTwice: two clients ask for the same article, not in the cache, at
// about the same time; the spool server answers the two requests (on two connections) with
// copies of different length, the longer one first. The article in the cache must be one of
// the two copies.
func TestR2ReproCacheSameArticleTwice(t *testing.T) {
	id := msgID(t)
	var n atomic.Int32
	copyOf := func(k int) []string { // what the k-th request is answered with
		pad := strings.Repeat("p", 20)
		if k == 1 {
			pad = strings.Repeat("p", 600)
		}
		var lines []string
		lines = append(lines, "Path: spool"+strconv.Itoa(k)+"!"+pad, "From: E2E Test <e2e@example.invalid>",
			"Newsgroups: e2e.test", "Subject: cached twice", "Message-ID: "+id, "Date: Mon, 02 Jan 2006 15:04:05 +0000", "")
		for i := range 8 {
			lines = append(lines, "copy "+strconv.Itoa(k)+" body "+strconv.Itoa(i))
		}
		return lines
	}
	handler := func(c *fakeserver.Conn) {
		for {
			line, err := c.ReadLine()
			if err != nil {
				return
			}
			f := strings.Fields(line)
			switch {
			case len(f) >= 2 && strings.EqualFold(f[0], "ARTICLE"):
				k := int(n.Add(1))
				all := copyOf(k)
				half := len(all) / 2
				c.WriteLines("220 0 " + f[1] + " article follows")
				c.WriteLines(all[:half]...)
				if k == 1 {
					c.Sleep(1500 * time.Millisecond)
				} else {
					c.Sleep(3 * time.Second)
				}
				c.WriteLines(all[half:]...)
				c.WriteLines(".")
			case len(f) > 0 && strings.EqualFold(f[0], "QUIT"):
				c.WriteLines("205 bye")
				return
			default:
				c.WriteLines("430 no")
			}
		}
	}
	// two spool servers: one connection each, so that the two requests are on two connections
	// (dreaderd queues a second request to a busy connection otherwise)
	sp1 := fakeserver.New(t, fakeserver.Options{Handler: handler})
	sp2 := fakeserver.New(t, fakeserver.Options{Addr: "127.0.0.2:0", Handler: handler})
	hosts := "127.0.0.1\ts1p" + strconv.Itoa(sp1.Port()) + "\n127.0.0.2\ts1p" + strconv.Itoa(sp2.Port()) + "\n"
	s := NewReaderSite(t, ReaderOptions{SpoolAddr: sp1.Addr(), Config: map[string]string{"readercache": "on"},
		Files: map[string]string{"dserver.hosts": hosts}})

	a := r2Dial(t, s.Addr)
	b := r2Dial(t, s.Addr)
	a.send("ARTICLE " + id + "\r\n")
	deadline := time.Now().Add(20 * time.Second)
	for sp1.Count("ARTICLE")+sp2.Count("ARTICLE") < 1 && time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
	}
	b.send("ARTICLE " + id + "\r\n")
	read := func(x *r2Raw) (string, []string) {
		st, err := x.line(60 * time.Second)
		if err != nil {
			t.Fatalf("status line: %v", err)
		}
		lines, err := x.block(60 * time.Second)
		if err != nil {
			t.Fatalf("article: %v (%q)", err, lines)
		}
		return st, lines
	}
	stA, la := read(a)
	stB, lb := read(b)
	t.Logf("client A: %q, %d lines; client B: %q, %d lines; spool saw %d ARTICLE", stA, len(la), stB, len(lb), sp1.Count("ARTICLE")+sp2.Count("ARTICLE"))
	time.Sleep(2 * time.Second)

	c := r2Dial(t, s.Addr) // from the cache now
	c.send("ARTICLE " + id + "\r\n")
	stC, err := c.line(30 * time.Second)
	if err != nil {
		t.Fatalf("third client, status line: %v", err)
	}
	lc, err := c.block(30 * time.Second)
	t.Logf("third client (cache): %q, %d lines, err %v", stC, len(lc), err)
	nul := false
	for _, l := range lc {
		if strings.ContainsRune(l, 0) {
			nul = true
		}
	}
	same := func(x, y []string) bool { return strings.Join(x, "\n") == strings.Join(y, "\n") }
	for i, l := range lc {
		t.Logf("cache line %02d: %.70q", i, l)
	}
	if err != nil || nul || !(same(lc, la) || same(lc, lb)) {
		t.Errorf("the article from the cache is neither copy: NUL %v, err %v\ncache: %q\nA:     %q\nB:     %q", nul, err, lc, la, lb)
	}
	t.Logf("general.log:\n%s", generalLog(s))
}

package e2e

// D-234 (deep review round 2, M6): two fetches of one article that is not in the reader cache,
// by two clients of one reader process, both wrote the cache's .tmp file: the fcntl lock that
// CreateCache takes belongs to the process, so the second fetch got it again and truncated and
// wrote the file under the first one. The cache then held a mix of the two copies (or a run of
// NUL bytes), served to every later reader until dexpirecache removed it.

import (
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-while/nntp-testkit/client"
	"github.com/go-while/nntp-testkit/fakeserver"
)

// TestReaderCacheSameArticleTwice: readercache on; two spool servers (127.0.0.1 and 127.0.0.2,
// one connection each, so the two requests run at the same time) answer the same Message-ID
// with copies of different length (the first request gets the long copy), each sent in two
// halves with a pause. Client A asks first, client B while A's copy is still coming. Both get
// their copy whole; a third client is then served from the cache (no third spool request) and
// gets exactly one of the two copies. Two orders: the first request ends first, and the first
// request ends last. Fails before the fix: the cached article was the short copy followed by
// the tail of the long one.
func TestReaderCacheSameArticleTwice(t *testing.T) {
	for _, tc := range []struct {
		name          string
		pause1, pause2 time.Duration
	}{
		{"first ends first", 1500 * time.Millisecond, 3 * time.Second},
		{"first ends last", 4 * time.Second, 1 * time.Second},
	} {
		t.Run(tc.name, func(t *testing.T) { cacheSameArticleTwice(t, tc.pause1, tc.pause2) })
	}
}

func cacheSameArticleTwice(t *testing.T, pause1, pause2 time.Duration) {
	id := msgID(t)
	var n atomic.Int32
	copyOf := func(k int) []string { // what the k-th request is answered with
		pad := strings.Repeat("p", 20)
		if k == 1 {
			pad = strings.Repeat("p", 600)
		}
		lines := []string{"Path: spool" + strconv.Itoa(k) + "!" + pad, "From: E2E Test <e2e@example.invalid>",
			"Newsgroups: e2e.test", "Subject: cached twice", "Message-ID: " + id,
			"Date: Mon, 02 Jan 2006 15:04:05 +0000", ""}
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
					c.Sleep(pause1)
				} else {
					c.Sleep(pause2)
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
	sp1 := fakeserver.New(t, fakeserver.Options{Handler: handler})
	sp2 := fakeserver.New(t, fakeserver.Options{Addr: "127.0.0.2:0", Handler: handler})
	hosts := "127.0.0.1\ts1p" + strconv.Itoa(sp1.Port()) + "\n127.0.0.2\ts1p" + strconv.Itoa(sp2.Port()) + "\n"
	s := NewReaderSite(t, ReaderOptions{SpoolAddr: sp1.Addr(), Config: map[string]string{"readercache": "on"},
		Files: map[string]string{"dserver.hosts": hosts}})
	asked := func() int { return sp1.Count("ARTICLE") + sp2.Count("ARTICLE") }

	a := s.Reader(t)
	b := s.Reader(t)
	a.MustSend("ARTICLE " + id + "\r\n")
	deadline := time.Now().Add(20 * time.Second)
	for asked() < 1 && time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
	}
	b.MustSend("ARTICLE " + id + "\r\n")
	read := func(who string, c *client.Conn) []string {
		code, line, err := c.ReadStatus()
		if err != nil || code != 220 {
			t.Fatalf("client %s: ARTICLE %s: %d %q %v", who, id, code, line, err)
		}
		lines, err := c.TryReadDotLines()
		if err != nil {
			t.Fatalf("client %s: article: %v (%q)", who, err, lines)
		}
		return lines
	}
	la := read("A", a)
	lb := read("B", b)
	if asked() != 2 {
		t.Fatalf("the spool servers were asked %d times, want 2 (one per client)", asked())
	}
	// dreaderd may put a cluster name in front of Path: (from the cache, see
	// TestReaderCacheHeaderRewrite): compare the Path: line from "spool" on
	join := func(x []string) string {
		y := append([]string(nil), x...)
		for i, l := range y {
			if strings.HasPrefix(l, "Path: ") {
				if j := strings.Index(l, "spool"); j >= 0 {
					y[i] = "Path: " + l[j:]
				}
				break
			}
		}
		return strings.Join(y, "\n")
	}
	if join(la) == join(lb) {
		t.Fatalf("both clients got the same copy; the test needs two different ones")
	}
	for who, l := range map[string][]string{"A": la, "B": lb} {
		k := map[string]string{"A": "1", "B": "2"}[who]
		if !strings.HasPrefix(join(l), "Path: spool"+k+"!") || !strings.HasSuffix(join(l), "copy "+k+" body 7") {
			t.Errorf("client %s did not get copy %s whole: %q", who, k, l)
		}
	}
	time.Sleep(1 * time.Second)

	c := s.Reader(t)
	c.MustSend("ARTICLE " + id + "\r\n")
	lc := read("C (cache)", c)
	if asked() != 2 {
		t.Errorf("the third client's ARTICLE went to a spool server (%d requests): the article was not cached", asked())
	}
	if strings.ContainsRune(join(lc), 0) || (join(lc) != join(la) && join(lc) != join(lb)) {
		t.Errorf("the article from the cache is neither copy:\ncache: %q\nA:     %q\nB:     %q", lc, la, lb)
	}
}

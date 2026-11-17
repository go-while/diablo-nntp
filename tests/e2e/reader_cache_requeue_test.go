package e2e

// D-234 (deep review round 2, found in its review): a fetch that has started its reader cache
// file and whose spool server goes away in the middle of the article header is requeued to the
// next server. That server's answer went into the same cache file after the first server's partial
// lines: with cache=off on the next server CreateCache was not called again, and the mix was
// committed and served to later readers. A requeue now drops the request's cache file.

import (
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-while/nntp-testkit/fakeserver"
)

// TestReaderCacheRequeueMidHeader: readercache on; spool server 1 (priority 1) answers the first
// ARTICLE with 220 and three header lines of its copy and then closes the connection (later
// ARTICLEs: 430); spool server 2 (priority 2) has the whole article with another Path. Client A
// asks (dreaderd requeues the request to server 2), then client C asks for the same article.
// "next server cache=off": C must get server 2's copy, not server 1's partial header in front of
// it. Fails before the fix: C was served the mix from the cache. "both cache on": C is served
// from the cache (no further spool request) with server 2's copy only.
func TestReaderCacheRequeueMidHeader(t *testing.T) {
	for _, tc := range []struct {
		name, opt string
		cached    bool
	}{
		{"next server cache=off", " cache=off", false},
		{"both cache on", "", true},
	} {
		t.Run(tc.name, func(t *testing.T) { cacheRequeueMidHeader(t, tc.opt, tc.cached) })
	}
}

func cacheRequeueMidHeader(t *testing.T, opt2 string, cached bool) {
	id := msgID(t)
	copyOf := func(k int) []string {
		lines := []string{"Path: spool" + strconv.Itoa(k) + "!" + strings.Repeat("p", 40),
			"From: E2E Test <e2e@example.invalid>", "Newsgroups: e2e.test", "Subject: requeued",
			"Message-ID: " + id, "Date: Mon, 02 Jan 2006 15:04:05 +0000", ""}
		for i := range 6 {
			lines = append(lines, "copy "+strconv.Itoa(k)+" body "+strconv.Itoa(i))
		}
		return lines
	}
	var n1, n2 atomic.Int32
	handler := func(k int, n *atomic.Int32) func(*fakeserver.Conn) {
		return func(c *fakeserver.Conn) {
			for {
				line, err := c.ReadLine()
				if err != nil {
					return
				}
				f := strings.Fields(line)
				switch {
				case len(f) >= 2 && strings.EqualFold(f[0], "ARTICLE"):
					nth := n.Add(1)
					if k == 1 && nth > 1 {
						c.WriteLines("430 no such article")
						continue
					}
					c.WriteLines("220 0 " + f[1] + " article follows")
					all := copyOf(k)
					if k == 1 { // three header lines, then the server goes away
						c.WriteLines(all[:3]...)
						c.Sleep(300 * time.Millisecond)
						return
					}
					c.WriteLines(all...)
					c.WriteLines(".")
				case len(f) > 0 && strings.EqualFold(f[0], "QUIT"):
					c.WriteLines("205 bye")
					return
				default:
					c.WriteLines("430 no")
				}
			}
		}
	}
	sp1 := fakeserver.New(t, fakeserver.Options{Handler: handler(1, &n1)})
	sp2 := fakeserver.New(t, fakeserver.Options{Addr: "127.0.0.2:0", Handler: handler(2, &n2)})
	hosts := "127.0.0.1\ts1p" + strconv.Itoa(sp1.Port()) + "\n127.0.0.2\ts2p" + strconv.Itoa(sp2.Port()) + opt2 + "\n"
	s := NewReaderSite(t, ReaderOptions{SpoolAddr: sp1.Addr(), Config: map[string]string{"readercache": "on"},
		Files: map[string]string{"dserver.hosts": hosts}})

	a := s.Reader(t)
	a.MustSend("ARTICLE " + id + "\r\n")
	// What A gets after the requeue is not this test's concern: read it and let it go.
	code, line, err := a.ReadStatus()
	var la []string
	if err == nil && code == 220 {
		la, err = a.TryReadDotLines()
	}
	t.Logf("client A: %d %q %v, %d lines", code, line, err, len(la))
	if n1.Load() != 1 || n2.Load() != 1 {
		t.Fatalf("spool requests: server 1 %d, server 2 %d; want 1 and 1 (A's request requeued from 1 to 2)", n1.Load(), n2.Load())
	}
	time.Sleep(1 * time.Second)

	before := n1.Load() + n2.Load()
	c := s.Reader(t)
	c.MustSend("ARTICLE " + id + "\r\n")
	code, line, err = c.ReadStatus()
	if err != nil || code != 220 {
		t.Fatalf("client C: ARTICLE %s: %d %q %v", id, code, line, err)
	}
	lc, err := c.TryReadDotLines()
	if err != nil {
		t.Fatalf("client C: article: %v (%q)", err, lc)
	}
	// server 2's copy, once: one Path: (from server 2) and one Message-ID: line, nothing of
	// server 1's copy, and server 2's body after the header
	paths, ids, blank := 0, 0, -1
	for i, l := range lc {
		if strings.Contains(l, "spool1") {
			t.Errorf("client C got a line of server 1's partial copy: %q", l)
		}
		if blank < 0 {
			switch {
			case strings.HasPrefix(l, "Path: "):
				paths++
				if !strings.Contains(l, "spool2!") {
					t.Errorf("client C: Path line not from server 2: %q", l)
				}
			case strings.HasPrefix(l, "Message-ID: "):
				ids++
			case l == "":
				blank = i
			}
		}
	}
	want := copyOf(2)
	wantBody := strings.Join(want[slices.Index(want, "")+1:], "\n")
	if paths != 1 || ids != 1 || blank < 0 || strings.Join(lc[blank+1:], "\n") != wantBody {
		t.Errorf("client C did not get server 2's copy once (Path lines %d, Message-ID lines %d):\n%q", paths, ids, lc)
	}
	if cached && n1.Load()+n2.Load() != before {
		t.Errorf("both cache on: C's ARTICLE went to a spool server (%d more requests): server 2's copy was not cached", n1.Load()+n2.Load()-before)
	}
}

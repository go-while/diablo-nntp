package e2e

import (
	"net"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	testkit "github.com/go-while/nntp-testkit"
	"github.com/go-while/nntp-testkit/client"
	"github.com/go-while/nntp-testkit/fakeserver"
)

// contentPoster records POST output at the outgoing peer, including the body.
func contentPoster(t *testing.T, access string) (*Site, func([]string, []string) []string) {
	t.Helper()
	_, spool := spoolStore(t)
	var mu sync.Mutex
	var arts [][]string
	peer := fakeserver.New(t, fakeserver.Options{Handler: fakeserver.PeerHandler(&fakeserver.PeerOptions{
		OnArticle: func(_ string, wire []string) {
			mu.Lock()
			defer mu.Unlock()
			// PeerHandler reports dot-stuffed wire lines. Return article content.
			art := slices.Clone(wire)
			for i, line := range art {
				if strings.HasPrefix(line, "..") {
					art[i] = line[1:]
				}
			}
			arts = append(arts, art)
		},
	})})
	s := NewReaderSite(t, ReaderOptions{Spool: spool, Post: peer, Access: access,
		Config: map[string]string{"path_generallog": "%s/general.log"}})
	return s, func(hdrs, body []string) []string {
		t.Helper()
		mu.Lock()
		n := len(arts)
		mu.Unlock()
		c := s.Reader(t)
		defer c.Close()
		c.ExpectCmd(340, "POST")
		art := []string{"From: Poster <poster@example.invalid>", "Newsgroups: e2e.test",
			"Subject: preserved content", "Message-ID: " + msgID(t)}
		c.MustSend(client.Stuff(slices.Concat(art, hdrs, []string{""}, body)))
		c.ExpectStatus(240, "POST")
		testkit.WaitFor(t, 10*time.Second, "POST at the outgoing peer", func() bool {
			mu.Lock()
			defer mu.Unlock()
			return len(arts) > n
		})
		mu.Lock()
		defer mu.Unlock()
		return slices.Clone(arts[n])
	}
}

// TestReaderPostLines (RFC 5536 3.3.1; RFC 5537 3.5 step 6): no generated
// Lines by default, and a supplied header is preserved in all modes. The
// explicit legacy mode may generate Lines when the poster did not supply it.
func TestReaderPostLines(t *testing.T) {
	body := []string{"first", "", ".last"}
	for _, mode := range []struct{ name, vs string }{
		{"default", ""},
		{"rfc5537 no", "    rfc5537 no\n"},
		{"legacytrace yes", "    legacytrace yes\n"},
	} {
		t.Run(mode.name, func(t *testing.T) {
			_, post := contentPoster(t, plainAccess(mode.vs))
			for _, own := range []string{"", "Lines: 99", "lInEs:\t00099"} {
				var hdrs []string
				if own != "" {
					hdrs = []string{own}
				}
				head, gotBody := splitArticle(post(hdrs, body))
				var lines []string
				for _, h := range head {
					if k, _, ok := strings.Cut(h, ":"); ok && strings.EqualFold(k, "Lines") {
						lines = append(lines, h)
					}
				}
				var want []string
				if own != "" {
					want = []string{own}
				} else if mode.vs != "" {
					want = []string{"Lines: 3"}
				}
				if !slices.Equal(lines, want) {
					t.Errorf("POST %q: Lines headers %q, want %q", own, lines, want)
				}
				if !slices.Equal(gotBody, body) {
					t.Errorf("body %q, want %q", gotBody, body)
				}
			}
		})
	}
}

// TestReaderPostTrailerIgnored: old access files still load, but no mode
// appends a trailer (RFC 5537 3.5 step 6). Warn once per access-file read,
// including repeated keywords and unused vserver definitions.
func TestReaderPostTrailerIgnored(t *testing.T) {
	for _, mode := range []struct{ name, vs string }{
		{"default", ""},
		{"rfc5537 no", "    rfc5537 no\n"},
		{"legacytrace yes", "    legacytrace yes\n"},
	} {
		t.Run(mode.name, func(t *testing.T) {
			access := plainAccess(mode.vs+"    posttrailer first\\nsecond\n    posttrailer last\\nfooter\n") +
				"vserverdef unused\n    posttrailer hidden\nend\n"
			s, post := contentPoster(t, access)
			for _, body := range [][]string{{"first", ".dot", "", "last"}, {"only\tline", ""}} {
				_, got := splitArticle(post(nil, body))
				if !slices.Equal(got, body) {
					t.Errorf("body %q, want unchanged %q", got, body)
				}
			}
			const warning = "posttrailer is no longer supported; ignored"
			if log := generalLog(s); strings.Count(log, warning) != 1 {
				t.Errorf("warning count %d, want 1:\n%s", strings.Count(log, warning), log)
				return
			}
			if mode.name == "default" {
				s.WriteFile(t, "dreader.access", access+"# reload the same obsolete options\n")
				testkit.WaitFor(t, 10*time.Second, "one warning for the reloaded file", func() bool {
					c := s.Reader(t)
					c.Close()
					return strings.Count(generalLog(s), warning) >= 2
				})
				for range 3 {
					c := s.Reader(t)
					c.Close()
				}
				if n := strings.Count(generalLog(s), warning); n != 2 {
					t.Errorf("warning count after one reload and repeated clients %d, want 2", n)
				}
			}
		})
	}
}

func TestReaderPostTrailerEmptyAccepted(t *testing.T) {
	s := NewReaderSite(t, ReaderOptions{SpoolAddr: "127.0.0.1:1",
		Access: plainAccess("    posttrailer\n"), Config: map[string]string{"path_generallog": "%s/general.log"}})
	if g := readerGreeting(s.Addr); !isService(g) {
		t.Errorf("empty obsolete option refused the access file: %q", g)
	}
	if n := strings.Count(generalLog(s), "posttrailer is no longer supported; ignored"); n != 1 {
		t.Errorf("warning count %d, want 1", n)
	}
}

// TestReaderPostLinesOverview: POST output without Lines, or with a misleading
// poster's Lines, still gets its actual body count through D-174's queue field
// and header feed. Capture at the posting peer, then replay into a real feeder.
func TestReaderPostLinesOverview(t *testing.T) {
	_, post := contentPoster(t, DefaultReaderAccess)
	arts := [][]string{post(nil, []string{"first", "", ".last"}),
		post([]string{"Lines: 99"}, []string{"one", "two", "three"})}
	feeds := GlobalLabel + "label peer\n    inhost " + PeerIP + "\n    alias e2e.peer\n    addgroup *\nend\n" +
		"label reader\n    inhost 127.0.0.1\n    hostname 127.0.0.1\n    alias dummy\n    headfeed on\n    addgroup *\nend\n"
	feeder := NewSite(t, SiteOptions{Newsfeeds: feeds})
	r := NewReaderSite(t, ReaderOptions{SpoolAddr: feeder.Addr})
	p := feeder.Peer(t)
	for _, art := range arts {
		head, _ := splitArticle(art)
		id := headerValue(head, "Message-ID")
		p.ExpectCmd(335, "IHAVE %s", id)
		p.MustSend(client.Stuff(art))
		p.ExpectStatus(235, "IHAVE")
	}
	p.ExpectCmd(205, "QUIT")
	for _, q := range feeder.waitQueue(t, "reader", 2) {
		if !strings.Contains(q+" ", " L3 ") {
			t.Errorf("queue %q, want L3", q)
		}
	}
	_, port, _ := net.SplitHostPort(r.Addr)
	feeder.Run(t, "dnewslink", "-b", "reader", "-h", "127.0.0.1", "-P", port, "-H", "-B", PeerIP)
	c := r.Reader(t)
	defer c.Close()
	waitGroupCount(t, c, "2")
	checkLines(t, c, map[string]string{"1": "3", "2": "3"})
}

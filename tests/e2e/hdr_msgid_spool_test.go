package e2e

import (
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	testkit "github.com/go-while/nntp-testkit"
	"github.com/go-while/nntp-testkit/fakeserver"
)

// HDR/XHDR by message-id cover the spool, not just the selected group's
// overview (D-193). Neither command may change the group/current article.
func TestHdrMessageIdSpool(t *testing.T) {
	st, spool := spoolStore(t)
	s := NewReaderSite(t, ReaderOptions{Spool: spool, Config: map[string]string{"readercache": "on"}})
	current, other, spoolOnly := msgID(t), msgID(t), msgID(t)
	feedReader(t, s, st, "e2e.test", newArticle(current, "e2e.test", "current subject", "body"))
	feedReader(t, s, st, "e2e.other", newArticle(other, "e2e.other", "other subject", "body"))
	// Not fed into dreaderd: this article has no local overview anywhere.
	st.AddArticle("e2e.other", fakeserver.Article{MessageID: spoolOnly, Headers: []string{
		"Message-ID: " + spoolOnly, "Subject: spool-only subject",
		"X-Folded: first", "\tsecond", " third", ".Dot-Field: dot value",
	}, Body: []string{"X-Not-A-Header: body value"}})

	for _, selected := range []bool{false, true} {
		name := "no_group"
		if selected {
			name = "other_group_selected"
		}
		t.Run(name, func(t *testing.T) {
			c := s.Reader(t)
			defer c.ExpectCmd(205, "QUIT")
			if selected {
				c.ExpectCmd(211, "GROUP e2e.test")
				c.ExpectCmd(223, "STAT 1")
				n := spool.Count("HEAD")
				c.ExpectCmd(225, "HDR Subject %s", current)
				if got := c.ReadDotLines(); !slices.Equal(got, []string{"0 current subject"}) {
					t.Errorf("overview fast path: %q", got)
				}
				if got := spool.Count("HEAD"); got != n {
					t.Errorf("overview hit fetched HEAD: count %d -> %d", n, got)
				}
			}
			for _, tc := range []struct {
				command, want string
				code          int
			}{
				{"HDR Subject " + other, "0 other subject", 225},
				{"XHDR Subject " + other, other + " other subject", 221},
				{"HDR sUbJeCt " + spoolOnly, "0 spool-only subject", 225},
				{"XHDR Subject " + spoolOnly, spoolOnly + " spool-only subject", 221},
				{"HDR X-Folded " + spoolOnly, "0 first second third", 225},
				{"XHDR X-Folded " + spoolOnly, spoolOnly + " first second third", 221},
				{"HDR .Dot-Field " + spoolOnly, "0 dot value", 225},
				{"HDR X-Absent " + spoolOnly, "0 ", 225},
				{"XHDR X-Absent " + spoolOnly, spoolOnly + " (none)", 221},
				{"HDR X-Not-A-Header " + spoolOnly, "0 ", 225},
				{"HDR Subject <unknown-hdr@e2e.test>", "", 430},
				{"XHDR Subject <unknown-hdr@e2e.test>", "", 430},
				{"HDR Subject <>", "", 430},
				{"HDR :lines " + other, "", 503},
				{"HDR :bytes " + other, "", 503},
				{"XHDR :lines " + other, "", 503},
				{"XHDR :bytes " + other, "", 503},
				// OVER/XOVER remain limited to the current group.
				{"OVER " + other, "", 430},
				{"XOVER " + other, "", 430},
			} {
				code, line := c.Cmd("%s", tc.command)
				var got []string
				if code == 225 || code == 221 || code == 224 {
					got = c.ReadDotLines()
				}
				if code != tc.code {
					t.Errorf("%s: %s, want %d", tc.command, line, tc.code)
				} else if tc.code < 400 && !slices.Equal(got, []string{tc.want}) {
					t.Errorf("%s: %q, want [%q]", tc.command, got, tc.want)
				}
			}
			// A fallback HEAD must never become a body-less cache entry.
			c.ExpectCmd(220, "ARTICLE %s", spoolOnly)
			_, body := splitArticle(c.ReadDotLines())
			if !slices.Equal(body, []string{"X-Not-A-Header: body value"}) {
				t.Errorf("ARTICLE after HDR: body %q", body)
			}
			if selected {
				_, line := c.Cmd("STAT")
				if !strings.HasPrefix(line, "223 1 "+current) {
					t.Errorf("current article changed: %s", line)
				}
				c.ExpectCmd(225, "HDR Subject 1")
				if got := c.ReadDotLines(); !slices.Equal(got, []string{"1 current subject"}) {
					t.Errorf("selected group changed: %q", got)
				}
			} else {
				c.ExpectCmd(412, "STAT")
			}
		})
	}
	if spool.Count("HEAD") == 0 {
		t.Error("no spool HEAD requests recorded")
	}
}

// A backend can pause mid-HEAD without blocking any other client of the same
// reader fork. A pipelined command runs only after the complete HDR response.
func TestHdrMessageIdSpoolAsync(t *testing.T) {
	for _, disconnect := range []bool{false, true} {
		name := "pipeline"
		if disconnect {
			name = "disconnect"
		}
		t.Run(name, func(t *testing.T) {
			seen, release := make(chan struct{}, 1), make(chan struct{})
			var once sync.Once
			unblock := func() { once.Do(func() { close(release) }) }
			defer unblock()
			spool := fakeserver.New(t, fakeserver.Options{Handler: func(c *fakeserver.Conn) {
				for {
					line, err := c.ReadLine()
					if err != nil {
						return
					}
					if strings.HasPrefix(strings.ToLower(line), "head ") {
						if c.WriteLines("221 headers follow", "Subject: first") != nil {
							return
						}
						select {
						case seen <- struct{}{}:
						default:
						}
						<-release
						if c.WriteLines("\tsecond", ".") != nil {
							return
						}
					} else if c.WriteLines("500 unsupported") != nil {
						return
					}
				}
			}})
			// Even a localspool backend uses asynchronous HEAD here, not the
			// synchronous local ARTICLE/DFA path. There is one reader fork.
			s := NewReaderSite(t, ReaderOptions{Spool: spool, Files: map[string]string{
				"dserver.hosts": "127.0.0.1\ts1p" + strings.TrimPrefix(spool.Addr(), "127.0.0.1:") + " localspool=" + t.TempDir() + "\n",
			}})
			pending, other := s.Reader(t), s.Reader(t)
			pending.MustSend("HDR Subject <async-hdr@e2e.test>\r\nDATE\r\n")
			select {
			case <-seen:
			case <-time.After(testkit.Deadline()):
				t.Fatal("reader did not request spool HEAD")
			}
			if disconnect {
				pending.Close()
			}
			// The backend has not finished; this cannot pass if its read blocks.
			other.ExpectCmd(111, "DATE")
			unblock()
			if !disconnect {
				pending.ExpectStatus(225, "pending HDR")
				if got := pending.ReadDotLines(); !slices.Equal(got, []string{"0 first second"}) {
					t.Errorf("fragmented HEAD: %q", got)
				}
				pending.ExpectStatus(111, "pipelined DATE")
				pending.ExpectCmd(205, "QUIT")
			}
			other.ExpectCmd(225, "HDR Subject <async-hdr@e2e.test>")
			if got := other.ReadDotLines(); !slices.Equal(got, []string{"0 first second"}) {
				t.Errorf("backend stream after pending client: %q", got)
			}
			other.ExpectCmd(205, "QUIT")
		})
	}
}

// An oversized HEAD is refused without sending a partial positive response or
// losing alignment on the spool connection; a subsequent normal HEAD succeeds.
func TestHdrMessageIdSpoolBound(t *testing.T) {
	big := []string{"221 oversized headers follow"}
	for range 140 {
		big = append(big, "X-Padding: "+strings.Repeat("x", 8192))
	}
	big = append(big, "Subject: too big", ".")
	spool := fakeserver.New(t, fakeserver.Options{Faults: fakeserver.Faults{
		OverrideLine: map[string][]string{
			"HEAD <big-hdr@e2e.test>":   big,
			"HEAD <small-hdr@e2e.test>": {"221 headers follow", "Subject: small", "."},
		},
	}, Store: fakeserver.NewStore()})
	s := NewReaderSite(t, ReaderOptions{Spool: spool})
	c := s.Reader(t)
	c.ExpectCmd(503, "HDR Subject <big-hdr@e2e.test>")
	c.ExpectCmd(225, "HDR Subject <small-hdr@e2e.test>")
	if got := c.ReadDotLines(); !slices.Equal(got, []string{"0 small"}) {
		t.Errorf("HEAD after overflow: %q", got)
	}
	c.ExpectCmd(205, "QUIT")
}

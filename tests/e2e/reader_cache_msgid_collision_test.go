package e2e

// D-211 (reader cache): dreaderd found a reader-cache file by the hash of the Message-ID alone
// and served it without looking at the Message-ID in it. With msgidhash hash (crc, the default)
// the pair "<1510900044@gn>"/"<1510900045@gn>" has one hash (D-210, TestMsgIdHashNeighbours):
// once the first was cached, a request for the second got "220 0 <second>" with the first one's
// article from the cache, and a request for a Message-ID the spool server does not have got the
// other article instead of 430. dreaderd now compares the cached article's Message-ID: header
// with the requested one, and a file that holds the other article is a miss. This covers the
// reader cache only: a Diablo spool server with the same hash also looks articles up by the
// hash alone, so the test uses a fake spool server that answers by the exact Message-ID.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	testkit "github.com/go-while/nntp-testkit"
	"github.com/go-while/nntp-testkit/client"
	"github.com/go-while/nntp-testkit/fakeserver"
)

// TestReaderCacheMsgIdCollision: readercache on, msgidhash hash (the default, set so the test
// keeps its meaning if the default changes); the spool is a fake server that answers ARTICLE
// for the articles it holds and 430 for any other Message-ID. "<1510900044@gn>" is fetched
// first (220, its own article) and cached: one cache file. Then, on the same connection:
//   - spool holds both, and "<1510900045@gn>" was fed to dreaderd (number 1 in e2e.test):
//     ARTICLE, HEAD and BODY of "<1510900045@gn>" get its own article (status line, Message-ID:
//     and Subject: headers, body), and ARTICLE 1 and BODY 1 its own body, each asked for at the
//     spool server; "<1510900044@gn>" is still served from the cache (no spool request), its own.
//   - spool holds only the first: STAT, HEAD and ARTICLE of "<1510900045@gn>" are 430, each
//     asked for at the spool server.
//   - spool holds only the first, stored with a bare CR between "Message-ID:" and the value
//     (MsgId() skips it, so dreaderd takes such an article): ARTICLE "<1510900044@gn>" again is
//     served from the cache (no spool request).
//   - spool holds only the first, stored with its Message-ID as a folded, lowercase header with a
//     comment after the value ("message-id:", then "\t<id> (comment)"): ARTICLE "<1510900044@gn>"
//     again is served from the cache; so it is after the test has rewritten the cache file with
//     LF line ends (a spool server cannot deliver those, see TestCachedLFSeparator).
//
// In each, the cache directory keeps the one file, with the same inode, size and modification
// time (in the last case, as the test rewrote it), and no *.tmp file: the file of the other
// article is neither replaced nor changed.
// Fails before the fix: "<1510900045@gn>" was answered 220, 221, 222 and 223 from the cache file
// of "<1510900044@gn>" (its Message-ID: header and body), ARTICLE 1 and BODY 1 got the body of
// "<1510900044@gn>", and the spool server was not asked. The bare-CR and header-form cases pass
// before the fix (there was no check); they show the check does not turn such hits into misses.
func TestReaderCacheMsgIdCollision(t *testing.T) {
	ids := []string{"<1510900044@gn>", "<1510900045@gn>"}
	subjects := []string{"first of the pair", "second of the pair"}
	bodies := []string{"body of the first of the pair", "body of the second of the pair"}
	for _, tc := range []struct {
		name   string
		held   int  // how many of ids the spool server holds (the first n)
		bareCR bool // the first is stored as "Message-ID:\r <id>"
		forms  bool // the first is stored as "message-id:" folded, "\t<id> (comment)"
	}{
		{"spool holds both", 2, false, false},
		{"spool holds the first only", 1, false, false},
		{"bare CR before the Message-ID", 1, true, false},
		{"folded lowercase header with a comment, LF lines", 1, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			arts := map[string][]string{}
			for i := 0; i < tc.held; i++ {
				arts[ids[i]] = newArticle(ids[i], "e2e.test", subjects[i], bodies[i])
			}
			if tc.bareCR || tc.forms {
				var head []string
				for _, h := range arts[ids[0]] {
					switch {
					case !strings.HasPrefix(h, "Message-ID: "):
						head = append(head, h)
					case tc.bareCR:
						head = append(head, "Message-ID:\r "+ids[0])
					default:
						head = append(head, "message-id:", "\t"+ids[0]+" (comment)")
					}
				}
				arts[ids[0]] = head
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
						art, ok := arts[f[1]]
						if !ok {
							c.WriteLines("430 No such article")
							continue
						}
						reply := append([]string{"220 0 " + f[1] + " article follows"}, art...)
						c.WriteLines(append(reply, ".")...)
					case len(f) > 0 && strings.EqualFold(f[0], "MODE"):
						c.WriteLines("200 reader mode")
					case len(f) > 0 && strings.EqualFold(f[0], "QUIT"):
						c.WriteLines("205 bye")
						return
					default:
						c.WriteLines("430 no")
					}
				}
			}
			sp := fakeserver.New(t, fakeserver.Options{Handler: handler})
			s := NewReaderSite(t, ReaderOptions{Spool: sp,
				Config: map[string]string{"readercache": "on", "msgidhash": "hash"}})
			asked := func() int { return sp.Count("ARTICLE") }
			if tc.held == 2 {
				// the second of the pair gets a number in e2e.test, for ARTICLE 1 and BODY 1
				art := newArticle(ids[1], "e2e.test", subjects[1], bodies[1])
				p := s.dialFrom(t, PeerIP)
				p.ExpectCmd(335, "IHAVE %s", ids[1])
				p.MustSend(client.Stuff(art))
				p.ExpectStatus(235, "IHAVE "+ids[1])
				p.ExpectCmd(205, "QUIT")
			}

			c := s.Reader(t)
			defer c.ExpectCmd(205, "QUIT")
			if tc.bareCR || tc.forms {
				c.Strict = false // a bare CR, or later LF lines, passed on as stored
			}

			// fetchAs sends cmd and checks the reply code; on 220/221/222 it checks the Message-ID
			// id in the status line and returns the lines that follow
			fetchAs := func(cmd, id string, want int) []string {
				t.Helper()
				code, line := c.Cmd("%s", cmd)
				var lines []string
				if code == 220 || code == 221 || code == 222 {
					lines = c.ReadDotLines()
				}
				if code != want {
					t.Errorf("%s: %d %q, want %d", cmd, code, line, want)
					return nil
				}
				if f := strings.Fields(line); want != 430 && (len(f) < 3 || f[2] != id) {
					t.Errorf("%s: status line %q, want the Message-ID %s", cmd, line, id)
				}
				return lines
			}
			fetch := func(cmd, id string, want int) []string {
				t.Helper()
				return fetchAs(cmd+" "+id, id, want)
			}
			// checkHead checks the Message-ID: and Subject: headers of head against article i
			checkHead := func(what string, head []string, i int) {
				t.Helper()
				if got := headerValue(head, "Message-ID"); got != ids[i] && !tc.forms { // headerValue reads no folded value
					t.Errorf("%s: Message-ID: %q, want %q (subject %q)", what, got, ids[i], headerValue(head, "Subject"))
				}
				if got := headerValue(head, "Subject"); got != subjects[i] {
					t.Errorf("%s: Subject: %q, want %q", what, got, subjects[i])
				}
			}
			checkBody := func(what string, body []string, i int) {
				t.Helper()
				if len(body) != 1 || body[0] != bodies[i] {
					t.Errorf("%s: body %q, want [%q]", what, body, bodies[i])
				}
			}

			head, body := splitArticle(fetch("ARTICLE", ids[0], 220))
			checkHead("ARTICLE "+ids[0], head, 0)
			checkBody("ARTICLE "+ids[0], body, 0)
			testkit.WaitFor(t, testkit.Deadline(), "the cache file of "+ids[0], func() bool {
				return cacheFiles(t, s) == 1
			})
			// The state below is taken at least 1 s after the file was written, and every fetch that
			// could write it comes later (the last is checked after another second): a rewrite
			// would show a later modification time even on a filesystem that counts whole seconds.
			time.Sleep(1 * time.Second)
			cached := cacheDirState(t, s)
			if len(cached.files) != 1 || len(cached.tmp) != 0 {
				t.Fatalf("cache directory after ARTICLE %s: %d committed files, *.tmp %q; want 1 and none", ids[0], len(cached.files), cached.tmp)
			}
			for p := range cached.files {
				t.Logf("cache file of %s: %s", ids[0], filepath.Base(p))
			}
			n := asked()

			if tc.held == 2 {
				head, body = splitArticle(fetch("ARTICLE", ids[1], 220))
				checkHead("ARTICLE "+ids[1], head, 1)
				checkBody("ARTICLE "+ids[1], body, 1)
				checkHead("HEAD "+ids[1], fetch("HEAD", ids[1], 221), 1)
				checkBody("BODY "+ids[1], fetch("BODY", ids[1], 222), 1)
				if got := asked(); got != n+3 {
					t.Errorf("ARTICLE, HEAD and BODY %s: %d spool requests, want %d: served from the cache file of %s", ids[1], got-n, 3, ids[0])
				}
				n = asked()
				testkit.WaitFor(t, testkit.Deadline(), ids[1]+" in GROUP e2e.test", func() bool {
					_, line := c.Cmd("GROUP e2e.test")
					f := strings.Fields(line)
					return len(f) > 1 && f[0] == "211" && f[1] == "1"
				})
				_, body = splitArticle(fetchAs("ARTICLE 1", ids[1], 220))
				checkBody("ARTICLE 1 ("+ids[1]+")", body, 1)
				checkBody("BODY 1 ("+ids[1]+")", fetchAs("BODY 1", ids[1], 222), 1)
				if got := asked(); got != n+2 {
					t.Errorf("ARTICLE 1 and BODY 1 (%s): %d spool requests, want %d: served from the cache file of %s", ids[1], got-n, 2, ids[0])
				}
				n = asked()
				head, body = splitArticle(fetch("ARTICLE", ids[0], 220))
				checkHead("ARTICLE "+ids[0]+" again", head, 0)
				checkBody("ARTICLE "+ids[0]+" again", body, 0)
				if got := asked(); got != n {
					t.Errorf("ARTICLE %s again: %d spool requests, want none: not served from the cache", ids[0], got-n)
				}
			} else if tc.forms {
				head, body = splitArticle(fetch("ARTICLE", ids[0], 220))
				checkHead("ARTICLE "+ids[0]+" again", head, 0)
				checkBody("ARTICLE "+ids[0]+" again", body, 0)
				if got := asked(); got != n {
					t.Errorf("ARTICLE %s again (folded lowercase header with a comment): %d spool requests, want none: not served from the cache", ids[0], got-n)
				}
				// A spool server cannot deliver an article with LF line ends into the cache
				// (TestCachedLFSeparator): rewrite the cache file with LF line ends, as an older
				// cache may hold it, and ask again.
				for f := range cached.files {
					data, err := os.ReadFile(f)
					if err != nil {
						t.Fatal(err)
					}
					lf := strings.ReplaceAll(string(data), "\r\n", "\n")
					if lf == string(data) || !strings.Contains(lf, "\nmessage-id:\n\t"+ids[0]+" (comment)\n") {
						t.Fatalf("cache file %s does not hold the folded header with CRLF line ends:\n%q", f, data)
					}
					if err := os.WriteFile(f, []byte(lf), 0o644); err != nil {
						t.Fatal(err)
					}
				}
				cached = cacheDirState(t, s)
				_, body = splitArticle(fetch("ARTICLE", ids[0], 220))
				checkBody("ARTICLE "+ids[0]+" from the LF cache file", body, 0)
				if got := asked(); got != n {
					t.Errorf("ARTICLE %s from the LF cache file: %d spool requests, want none: not served from the cache", ids[0], got-n)
				}
			} else if tc.bareCR {
				head, body = splitArticle(fetch("ARTICLE", ids[0], 220))
				checkHead("ARTICLE "+ids[0]+" again", head, 0)
				checkBody("ARTICLE "+ids[0]+" again", body, 0)
				if got := asked(); got != n {
					t.Errorf("ARTICLE %s again (bare CR before the value): %d spool requests, want none: not served from the cache", ids[0], got-n)
				}
			} else {
				fetch("STAT", ids[1], 430)
				fetch("HEAD", ids[1], 430)
				fetch("ARTICLE", ids[1], 430)
				if got := asked(); got != n+3 {
					t.Errorf("STAT, HEAD and ARTICLE %s: %d spool requests, want %d: answered from the cache file of %s", ids[1], got-n, 3, ids[0])
				}
			}

			time.Sleep(1 * time.Second)
			if after := cacheDirState(t, s); len(after.files) == 2 {
				t.Fatalf("two cache files after the fetches of %s and %s: they do not share a hash here, so this test shows nothing", ids[0], ids[1])
			}
			cacheUntouched(t, s, cached, "after the requests for "+ids[1])
		})
	}
}

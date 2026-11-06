package e2e

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/go-while/nntp-testkit/client"
	"github.com/go-while/nntp-testkit/fakeserver"
)

// refeedLines reads a queue file dspoolrefeed wrote under the site directory.
func refeedLines(t *testing.T, s *Site, rel string) []string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(s.Dir, rel))
	if err != nil {
		t.Fatal(err)
	}
	return strings.FieldsFunc(string(b), func(r rune) bool { return r == '\n' })
}


// TestDspoolrefeed (F-03): dspoolrefeed writes, for every article on the spool, the queue line
// diablo itself wrote when the article came in (with C<size> on a compressed spool), -g selects
// by newsgroup, -n splits the output, an existing -o file is not overwritten, and dnewslink
// sends the articles from the written queue.
func TestDspoolrefeed(t *testing.T) {
	for _, tc := range []struct{ name, ctl string }{{"plain", ""}, {"compressed", compressedSpoolCtl}} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.ctl != "" {
				skipWithoutZlib(t)
			}
			s := NewSite(t, SiteOptions{Newsfeeds: OutNewsfeeds, SpoolCtl: tc.ctl})
			ids := append(s.Inject(t, "e2e.test", 2), s.Inject(t, "e2e.other", 1)...)
			// a Message-ID header with a comment after the <...>: the history hashes the
			// <...> part, and so must dspoolrefeed
			cid := msgID(t)
			art := newArticle(cid, "e2e.test", "comment after the Message-ID", "body")
			for i, l := range art {
				if strings.HasPrefix(l, "Message-ID: ") {
					art[i] = l + " (a comment)"
				}
			}
			p := s.Peer(t)
			p.ExpectCmd(335, "IHAVE %s", cid)
			p.MustSend(client.Stuff(art))
			p.ExpectStatus(235, "IHAVE with a comment")
			p.ExpectCmd(205, "QUIT")
			ids = append(ids, cid)
			want := s.waitQueue(t, OutLabel, 4)
			if tc.ctl != "" && !strings.Contains(want[0], " C") {
				t.Fatalf("diablo's queue line on a compressed spool has no C<size>: %q", want[0])
			}

			out := s.Run(t, "dspoolrefeed", "-o", "dqueue/refeed")
			if got := refeedLines(t, s, "dqueue/refeed"); !slices.Equal(sorted(got), sorted(want)) {
				t.Errorf("dspoolrefeed wrote\n%q\nwant diablo's queue lines\n%q\n%s", got, want, out)
			}
			if !strings.Contains(out, "4 written") {
				t.Errorf("dspoolrefeed counts: %s", out)
			}

			s.Run(t, "dspoolrefeed", "-q", "-g", "e2e.*,!e2e.other", "-o", "dqueue/sel")
			if got := refeedLines(t, s, "dqueue/sel"); len(got) != 3 || strings.Contains(strings.Join(got, "\n"), ids[2]) {
				t.Errorf("-g e2e.*,!e2e.other: %q, want the three e2e.test articles", got)
			}

			s.Run(t, "dspoolrefeed", "-q", "-n", "2", "-o", "dqueue/part")
			if a, b := refeedLines(t, s, "dqueue/part.0000"), refeedLines(t, s, "dqueue/part.0001"); len(a) != 2 || len(b) != 2 {
				t.Errorf("-n 2: part.0000 %q, part.0001 %q, want 2 and 2 lines", a, b)
			}

			if out, err := s.TryRun("dspoolrefeed", "-q", "-o", "dqueue/refeed"); err == nil {
				t.Errorf("dspoolrefeed -o onto an existing file succeeded:\n%s", out)
			}

			peer, got := newPeer(t, &fakeserver.PeerOptions{})
			dnl := s.dnewslink(t, peer, "refeed")
			if !slices.Equal(sorted(got.list()), sorted(ids)) {
				t.Errorf("peer got %q from the refeed queue, want %q\ndnewslink:\n%s", got.list(), ids, dnl)
			}
		})
	}
}

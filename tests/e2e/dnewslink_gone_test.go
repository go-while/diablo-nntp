package e2e

// dnewslink and queued articles that are no longer in the spool: a feeder working
// through a long backlog finds many of them expired. They must not be offered, and
// never sent as an empty article (a 3-byte "TooSmall" reject of a good message-id).

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/go-while/nntp-testkit/fakeserver"
)

// goneQueue injects three articles and rewrites dqueue/OutLabel so that they alternate
// with queue entries for gone articles: entries (at the same offsets) in a spool file
// that existed and was removed, as dexpire removes one. It returns the injected ids and
// the gone ones.
func goneQueue(t *testing.T, s *Site, extra func(q []string) []string) (ids, gone []string) {
	t.Helper()
	return spoiledQueue(t, s, extra, func(dir string) {
		if err := os.RemoveAll(dir); err != nil { // expired
			t.Fatal(err)
		}
	})
}

// spoiledQueue is goneQueue with spoil doing to the other spool file's directory what
// makes its entries unreadable.
func spoiledQueue(t *testing.T, s *Site, extra func(q []string) []string, spoil func(dir string)) (ids, gone []string) {
	t.Helper()
	ids = s.Inject(t, "e2e.test", 3)
	q := s.waitQueue(t, OutLabel, 3)
	rel := strings.Fields(q[0])[0]
	spool := filepath.Join(s.Dir, "spool", "news")
	data, err := os.ReadFile(filepath.Join(spool, rel))
	if err != nil {
		t.Fatal(err)
	}
	const goneRel = "D.00000001/B.0001"
	if err := os.MkdirAll(filepath.Join(spool, "D.00000001"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(spool, goneRel), data, 0o644); err != nil {
		t.Fatal(err)
	}
	var lines []string
	for i, l := range q {
		f := strings.Fields(l)
		lines = append(lines, l)
		id := "<gone" + strconv.Itoa(i) + "@e2e.test>"
		gone = append(gone, id)
		lines = append(lines, strings.Join(append([]string{goneRel, id}, f[2:]...), " "))
	}
	if extra != nil {
		lines = extra(lines)
	}
	spoil(filepath.Join(spool, "D.00000001"))
	s.WriteFile(t, filepath.Join("dqueue", OutLabel), strings.Join(lines, "\n")+"\n")
	return ids, gone
}

// checkGone checks what the peer got: the articles in want, whole, and nothing for the
// ids in skip, neither a command nor an (empty) article.
func checkGone(t *testing.T, s *Site, peer *fakeserver.Server, got *collector, want, skip []string, out string) {
	t.Helper()
	if !slices.Equal(got.list(), want) {
		t.Errorf("peer accepted %q, want %q\ndnewslink output:\n%s", got.list(), want, out)
	}
	arts := peer.Articles()
	if len(arts) != len(want) {
		t.Errorf("peer got %d articles, want %d", len(arts), len(want))
	}
	for i, a := range arts {
		w := strings.Join(a, "\n")
		if len(a) == 0 || !strings.Contains(w, "\nbody of <") {
			t.Errorf("article %d sent without its headers and body (%d lines): %q", i, len(a), w)
		}
	}
	for _, l := range peer.Lines() {
		for _, id := range skip {
			if strings.Contains(l, id) {
				t.Errorf("peer got %q for an article that is not in the spool", l)
			}
		}
	}
	for _, f := range s.queueFiles(t) {
		for _, id := range skip {
			if strings.Contains(f, id) {
				t.Errorf("%s is still queued:\n%s", id, f)
			}
		}
	}
}

// queueFiles returns the contents of every file in dqueue.
func (s *Site) queueFiles(t *testing.T) []string {
	t.Helper()
	ents, err := os.ReadDir(filepath.Join(s.Dir, "dqueue"))
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, e := range ents {
		if b, err := os.ReadFile(filepath.Join(s.Dir, "dqueue", e.Name())); err == nil {
			out = append(out, string(b))
		}
	}
	return out
}

// wantGoneLog checks the final stats line for articles not read from the spool.
func wantGoneLog(t *testing.T, out string, gone, bad, spoolerr int) {
	t.Helper()
	re := regexp.MustCompile(`final secs=\d+ +gone=` + strconv.Itoa(gone) + ` +bad=` + strconv.Itoa(bad) +
		` +spoolerr=` + strconv.Itoa(spoolerr) + `\b`)
	if !re.MatchString(out) {
		t.Errorf("dnewslink did not log final gone=%d bad=%d spoolerr=%d:\n%s", gone, bad, spoolerr, out)
	}
}

// TestDnewslinkGoneStreaming: a streaming peer gets CHECK and TAKETHIS only for the
// articles still in the spool; the gone ones are counted and dropped from the queue.
func TestDnewslinkGoneStreaming(t *testing.T) {
	peer, got := newPeer(t, &fakeserver.PeerOptions{})
	s := NewSite(t, SiteOptions{Newsfeeds: OutNewsfeeds})
	ids, gone := goneQueue(t, s, nil)
	out := s.dnewslink(t, peer, OutLabel, "-d")
	checkGone(t, s, peer, got, ids, gone, out)
	if n := peer.Count("TAKETHIS"); n != 3 {
		t.Errorf("TAKETHIS sent %d times, want 3 (counts %v)", n, peer.Counts())
	}
	wantGoneLog(t, out, 3, 0, 0)
}

// TestDnewslinkGoneIHAVE: the same with a peer without streaming: no IHAVE for a gone
// article.
func TestDnewslinkGoneIHAVE(t *testing.T) {
	peer, got := newPeer(t, &fakeserver.PeerOptions{NoStream: true})
	s := NewSite(t, SiteOptions{Newsfeeds: OutNewsfeeds})
	ids, gone := goneQueue(t, s, nil)
	out := s.dnewslink(t, peer, OutLabel, "-d")
	checkGone(t, s, peer, got, ids, gone, out)
	if n := peer.Count("IHAVE"); n != 3 {
		t.Errorf("IHAVE sent %d times, want 3 (counts %v)", n, peer.Counts())
	}
	wantGoneLog(t, out, 3, 0, 0)
}

// TestDnewslinkGoneNoCheck: with -I (TAKETHIS without CHECK) no TAKETHIS for a gone
// article.
func TestDnewslinkGoneNoCheck(t *testing.T) {
	peer, got := newPeer(t, &fakeserver.PeerOptions{})
	s := NewSite(t, SiteOptions{Newsfeeds: OutNewsfeeds})
	ids, gone := goneQueue(t, s, nil)
	out := s.dnewslink(t, peer, OutLabel, "-d", "-I")
	checkGone(t, s, peer, got, ids, gone, out)
	if n := peer.Count("CHECK"); n != 0 {
		t.Errorf("CHECK sent %d times with -I", n)
	}
	wantGoneLog(t, out, 3, 0, 0)
}

// TestDnewslinkGoneStat: -x (stat each spool file first) keeps working: it skips the gone
// articles and still sends the others.
func TestDnewslinkGoneStat(t *testing.T) {
	peer, got := newPeer(t, &fakeserver.PeerOptions{})
	s := NewSite(t, SiteOptions{Newsfeeds: OutNewsfeeds})
	ids, gone := goneQueue(t, s, nil)
	out := s.dnewslink(t, peer, OutLabel, "-d", "-x")
	checkGone(t, s, peer, got, ids, gone, out)
	wantGoneLog(t, out, 3, 0, 0)
}

// unreadable adds an entry whose spool file is there but whose article cannot be mapped
// (its size does not end on the article's terminator, as in a corrupted queue line): it
// passes the check before the offer and fails only when the article is to be sent.
func unreadable(id string) func(q []string) []string {
	return func(q []string) []string {
		f := strings.Fields(q[0])
		offSize := strings.SplitN(f[2], ",", 2)
		size, _ := strconv.Atoi(offSize[1])
		bad := strings.Join(append([]string{f[0], id, offSize[0] + "," + strconv.Itoa(size-1)}, f[3:]...), " ")
		return append(q[:1], append([]string{bad}, q[1:]...)...)
	}
}

// TestDnewslinkUnreadableAfterCheck: an article that cannot be mapped once the peer has
// answered 238 to its CHECK gets no TAKETHIS (CHECK replies are advisory, RFC 4644 2.4.2)
// and certainly no empty article; the rest of the queue goes on.
func TestDnewslinkUnreadableAfterCheck(t *testing.T) {
	const bad = "<unreadable@e2e.test>"
	peer, got := newPeer(t, &fakeserver.PeerOptions{})
	s := NewSite(t, SiteOptions{Newsfeeds: OutNewsfeeds})
	ids, gone := goneQueue(t, s, unreadable(bad))
	out := s.dnewslink(t, peer, OutLabel, "-d")
	checked := false
	for _, l := range peer.Lines() {
		switch {
		case strings.EqualFold(l, "CHECK "+bad):
			checked = true
		case strings.Contains(l, bad):
			t.Errorf("peer got %q for an article that cannot be read", l)
		}
	}
	if !checked {
		t.Errorf("no CHECK %s: the entry should pass the check before the offer (lines %q)", bad, peer.Lines())
	}
	checkGone(t, s, peer, got, ids, gone, out)
	wantGoneLog(t, out, 3, 1, 0)
}

// TestDnewslinkUnreadableIHAVE: without streaming the article is mapped only after the
// peer's 335, so an offer the peer refuses costs no read. If it then cannot be read, IHAVE
// has no way to take the offer back: the peer gets an empty article (as innfeed sends), the
// entry is counted bad and dropped, and the rest of the queue goes on.
func TestDnewslinkUnreadableIHAVE(t *testing.T) {
	const bad = "<unreadable@e2e.test>"
	peer, got := newPeer(t, &fakeserver.PeerOptions{NoStream: true})
	s := NewSite(t, SiteOptions{Newsfeeds: OutNewsfeeds})
	ids, gone := goneQueue(t, s, unreadable(bad))
	out := s.dnewslink(t, peer, OutLabel, "-d")
	if want := append([]string{ids[0], bad}, ids[1:]...); !slices.Equal(got.list(), want) {
		t.Errorf("peer accepted %q, want %q", got.list(), want)
	}
	empty := 0
	for _, a := range peer.Articles() {
		if len(a) == 0 {
			empty++
		} else if !strings.Contains(strings.Join(a, "\n"), "\nbody of <") {
			t.Errorf("article sent without its headers and body: %q", a)
		}
	}
	if empty != 1 {
		t.Errorf("%d empty articles, want 1 (the unreadable one, after its 335)", empty)
	}
	for _, l := range peer.Lines() {
		for _, id := range gone {
			if strings.Contains(l, id) {
				t.Errorf("peer got %q for an article that is not in the spool", l)
			}
		}
	}
	for _, f := range s.queueFiles(t) {
		if strings.Contains(f, bad) {
			t.Errorf("%s is still queued:\n%s", bad, f)
		}
	}
	wantGoneLog(t, out, 3, 1, 0)
}

// TestDnewslinkIHAVERefusedNotRead: an IHAVE offer the peer refuses (435) does not read the
// article. The unreadable entry is offered like any other and refused, not counted bad;
// with -d2 no article is mapped at all.
func TestDnewslinkIHAVERefusedNotRead(t *testing.T) {
	const bad = "<unreadable@e2e.test>"
	s := NewSite(t, SiteOptions{Newsfeeds: OutNewsfeeds})
	have := map[string]bool{bad: true}
	peer, got := newPeer(t, &fakeserver.PeerOptions{NoStream: true, Have: have})
	ids, gone := goneQueue(t, s, unreadable(bad))
	for _, id := range ids {
		have[id] = true
	}
	out := s.dnewslink(t, peer, OutLabel, "-d2")
	if n := peer.Count("IHAVE"); n != 4 {
		t.Errorf("IHAVE sent %d times, want 4: the 3 articles and the unreadable entry (counts %v)", n, peer.Counts())
	}
	if len(got.list()) != 0 || len(peer.Articles()) != 0 {
		t.Errorf("the peer refused everything but got %q", got.list())
	}
	if strings.Contains(out, ">> map ") {
		t.Errorf("articles were mapped for offers the peer refused:\n%s", out)
	}
	for _, l := range peer.Lines() {
		for _, id := range gone {
			if strings.Contains(l, id) {
				t.Errorf("peer got %q for an article that is not in the spool", l)
			}
		}
	}
	wantGoneLog(t, out, 3, 0, 0)
}

// TestDnewslinkSpoolErrorKept: a spool file that is there but cannot be opened (mode 000:
// EACCES, as an I/O or permission fault) is not expiry. Its entries are not offered, are
// logged and counted as spoolerr, and stay in the queue; the others are sent.
func TestDnewslinkSpoolErrorKept(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root opens a file of mode 000")
	}
	for _, mode := range []string{"streaming", "ihave"} {
		t.Run(mode, func(t *testing.T) {
			peer, got := newPeer(t, &fakeserver.PeerOptions{NoStream: mode == "ihave"})
			s := NewSite(t, SiteOptions{Newsfeeds: OutNewsfeeds})
			ids, kept := spoiledQueue(t, s, nil, func(dir string) {
				if err := os.Chmod(filepath.Join(dir, "B.0001"), 0); err != nil {
					t.Fatal(err)
				}
			})
			out := s.dnewslink(t, peer, OutLabel, "-d")
			if !slices.Equal(got.list(), ids) {
				t.Errorf("peer accepted %q, want %q\ndnewslink output:\n%s", got.list(), ids, out)
			}
			for _, l := range peer.Lines() {
				for _, id := range kept {
					if strings.Contains(l, id) {
						t.Errorf("peer got %q for an article that cannot be read", l)
					}
				}
			}
			q := strings.Join(s.queueFiles(t), "")
			for _, id := range kept {
				if n := strings.Count(q, id); n != 1 {
					t.Errorf("%s is queued %d times, want 1; queue now:\n%s", id, n, q)
				}
			}
			for _, id := range ids {
				if strings.Contains(q, id) {
					t.Errorf("%s was sent but is still queued:\n%s", id, q)
				}
			}
			if !strings.Contains(out, "permission denied") && !strings.Contains(out, "Permission denied") {
				t.Errorf("the spool error is not logged:\n%s", out)
			}
			wantGoneLog(t, out, 0, 0, 3)
		})
	}
}

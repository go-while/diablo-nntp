package e2e

// dnewslink and queued articles that are no longer in the spool: a feeder working
// through a long backlog finds many of them expired. They must not be offered, and
// never sent as an empty article (a 3-byte "TooSmall" reject of a good message-id).

import (
	"os"
	"path/filepath"
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
	if err := os.RemoveAll(filepath.Join(spool, "D.00000001")); err != nil { // expired
		t.Fatal(err)
	}
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

func wantGoneLog(t *testing.T, out string, n int) {
	t.Helper()
	if want := strconv.Itoa(n) + " articles expired before they could be sent"; !strings.Contains(out, want) {
		t.Errorf("dnewslink did not log %q:\n%s", want, out)
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
	wantGoneLog(t, out, 3)
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
	wantGoneLog(t, out, 3)
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
	wantGoneLog(t, out, 3)
}

// TestDnewslinkGoneStat: -x (stat each spool file first) keeps working: it skips the gone
// articles and still sends the others.
func TestDnewslinkGoneStat(t *testing.T) {
	peer, got := newPeer(t, &fakeserver.PeerOptions{})
	s := NewSite(t, SiteOptions{Newsfeeds: OutNewsfeeds})
	ids, gone := goneQueue(t, s, nil)
	out := s.dnewslink(t, peer, OutLabel, "-d", "-x")
	checkGone(t, s, peer, got, ids, gone, out)
	wantGoneLog(t, out, 3)
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
	wantGoneLog(t, out, 4)
}

// TestDnewslinkUnreadableIHAVE: the same without streaming: the article is mapped before
// the IHAVE, so the peer never waits for an article dnewslink cannot send.
func TestDnewslinkUnreadableIHAVE(t *testing.T) {
	const bad = "<unreadable@e2e.test>"
	peer, got := newPeer(t, &fakeserver.PeerOptions{NoStream: true})
	s := NewSite(t, SiteOptions{Newsfeeds: OutNewsfeeds})
	ids, gone := goneQueue(t, s, unreadable(bad))
	out := s.dnewslink(t, peer, OutLabel, "-d")
	checkGone(t, s, peer, got, ids, append(gone, bad), out)
	wantGoneLog(t, out, 4)
}

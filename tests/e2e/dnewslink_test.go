package e2e

// Diablo as the client: dnewslink sends the outgoing queue to a fake peer.

import (
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-while/nntp-testkit/fakeserver"
)

// collector records the message-ids a fake peer accepted, in order.
type collector struct {
	mu  sync.Mutex
	ids []string
}

func (c *collector) add(id string, _ []string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ids = append(c.ids, id)
}

func (c *collector) list() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return slices.Clone(c.ids)
}

// newPeer starts a fake transit peer (nntp-testkit PeerHandler) that records what it accepts.
func newPeer(tb testing.TB, o *fakeserver.PeerOptions) (*fakeserver.Server, *collector) {
	tb.Helper()
	col := &collector{}
	o.OnArticle = col.add
	return fakeserver.New(tb, fakeserver.Options{Handler: fakeserver.PeerHandler(o)}), col
}

// OutLabel is the dnewsfeeds label of the outgoing feed in OutNewsfeeds; diablo queues the
// articles for it in dqueue/OutLabel.
const OutLabel = "fake"

// OutNewsfeeds is DefaultNewsfeeds plus the outgoing feed OutLabel, which gets every article
// whose Path does not already name e2e.peer.
const OutNewsfeeds = GlobalLabel + `label peer
    inhost ` + PeerIP + `
    alias e2e.peer
    addgroup *
end
label ` + OutLabel + `
    hostname 127.0.0.1
    alias e2e.fake
    addgroup *
end
`

// Inject feeds n new articles for group to the site with IHAVE from PeerIP and returns their
// message-ids in order.
func (s *Site) Inject(tb testing.TB, group string, n int) []string {
	tb.Helper()
	p := s.Peer(tb)
	ids := make([]string, n)
	for i := range ids {
		ids[i] = msgID(tb)
		expect(tb, p, 335, "IHAVE %s", ids[i])
		send(tb, p, wire(newArticle(ids[i], group, "injected "+strconv.Itoa(i+1), "body of "+ids[i])))
		status(tb, p, 235, "IHAVE "+ids[i])
	}
	expect(tb, p, 205, "QUIT")
	return ids
}

// queue flushes diablo's outgoing queues and returns the lines of dqueue/label; a missing
// file has none.
func (s *Site) queue(tb testing.TB, label string) []string {
	tb.Helper()
	s.Dicmd(tb, "flush")
	b, err := os.ReadFile(filepath.Join(s.Dir, "dqueue", label))
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		tb.Fatal(err)
	}
	return strings.FieldsFunc(string(b), func(r rune) bool { return r == '\n' })
}

// waitQueue flushes the outgoing queues until dqueue/label has n lines and returns them.
// The connection's process reports each accepted article to the diablo master, which
// writes the queue entry, so the last entries may come in after the peer's QUIT.
func (s *Site) waitQueue(tb testing.TB, label string, n int) []string {
	tb.Helper()
	var q []string
	waitFor(tb, 10*time.Second, strconv.Itoa(n)+" entries in dqueue/"+label, func() bool {
		q = s.queue(tb, label)
		return len(q) >= n
	})
	if len(q) != n {
		tb.Fatalf("dqueue/%s: %q, want %d entries", label, q, n)
	}
	return q
}

// dnewslink runs dnewslink for the queue of label against the fake peer.
func (s *Site) dnewslink(tb testing.TB, peer *fakeserver.Server, label string, args ...string) string {
	tb.Helper()
	args = append([]string{"-b", label, "-h", "127.0.0.1", "-P", strconv.Itoa(peer.Port())}, args...)
	return s.Run(tb, "dnewslink", args...)
}

// TestDnewslinkStreaming: dnewslink streams the queued articles to a peer that offers
// MODE STREAM, in order, without IHAVE.
func TestDnewslinkStreaming(t *testing.T) {
	peer, got := newPeer(t, &fakeserver.PeerOptions{})
	s := NewSite(t, SiteOptions{Newsfeeds: OutNewsfeeds})
	ids := s.Inject(t, "e2e.test", 3)
	s.waitQueue(t, OutLabel, 3)
	out := s.dnewslink(t, peer, OutLabel)
	if !slices.Equal(got.list(), ids) {
		t.Errorf("peer accepted %q, want %q\ndnewslink output:\n%s", got.list(), ids, out)
	}
	if n := peer.Count("TAKETHIS"); n != 3 {
		t.Errorf("TAKETHIS sent %d times, want 3 (counts %v)", n, peer.Counts())
	}
	if n := peer.Count("IHAVE"); n != 0 {
		t.Errorf("IHAVE sent %d times while streaming", n)
	}
}

// TestDnewslinkIHAVEFallback: a peer that refuses MODE STREAM gets the articles with IHAVE.
func TestDnewslinkIHAVEFallback(t *testing.T) {
	peer, got := newPeer(t, &fakeserver.PeerOptions{NoStream: true})
	s := NewSite(t, SiteOptions{Newsfeeds: OutNewsfeeds})
	ids := s.Inject(t, "e2e.test", 3)
	s.waitQueue(t, OutLabel, 3)
	out := s.dnewslink(t, peer, OutLabel)
	if !slices.Equal(got.list(), ids) {
		t.Errorf("peer accepted %q, want %q\ndnewslink output:\n%s", got.list(), ids, out)
	}
	if n := peer.Count("IHAVE"); n != 3 {
		t.Errorf("IHAVE sent %d times, want 3 (counts %v)", n, peer.Counts())
	}
}

// TestDnewslinkPeerHas: dnewslink does not send an article the peer already has (CHECK gets
// 438); the others are accepted.
func TestDnewslinkPeerHas(t *testing.T) {
	s := NewSite(t, SiteOptions{Newsfeeds: OutNewsfeeds})
	ids := s.Inject(t, "e2e.test", 3)
	peer, got := newPeer(t, &fakeserver.PeerOptions{Have: map[string]bool{ids[1]: true}})
	s.waitQueue(t, OutLabel, 3)
	out := s.dnewslink(t, peer, OutLabel)
	if want := []string{ids[0], ids[2]}; !slices.Equal(got.list(), want) {
		t.Errorf("peer accepted %q, want %q\ndnewslink output:\n%s", got.list(), want, out)
	}
	if n := peer.Count("TAKETHIS"); n != 2 {
		t.Errorf("TAKETHIS sent %d times, want 2 (counts %v)", n, peer.Counts())
	}
	if slices.Contains(peer.Lines(), "TAKETHIS "+ids[1]) {
		t.Errorf("TAKETHIS %s sent although the peer has it", ids[1])
	}
}

// TestDnewslinkBatchTemplatePercentLabel checks that a percent in a feed label
// is unescaped while the batch sequence conversion is expanded.
func TestDnewslinkBatchTemplatePercentLabel(t *testing.T) {
	const label = "feed%label"
	peer, got := newPeer(t, &fakeserver.PeerOptions{})
	newsfeeds := strings.Replace(OutNewsfeeds, "label "+OutLabel+"\n", "label "+label+"\n", 1)
	s := NewSite(t, SiteOptions{Newsfeeds: newsfeeds})
	ids := s.Inject(t, "e2e.test", 1)
	queue := s.waitQueue(t, label, 1)
	batch := filepath.Join(s.Dir, "dqueue", label+".S00007")
	if err := os.WriteFile(batch, []byte(strings.Join(queue, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	template := strings.ReplaceAll(label, "%", "%%") + ".S%05d"
	out := s.Run(t, "dnewslink", "-b", template, "-S", "7", "-h", "127.0.0.1", "-P", strconv.Itoa(peer.Port()))
	if !slices.Equal(got.list(), ids) {
		t.Errorf("peer accepted %q, want %q\ndnewslink output:\n%s", got.list(), ids, out)
	}
}

func TestDnewslinkRejectsInvalidBatchTemplate(t *testing.T) {
	s := NewSite(t, SiteOptions{NoStart: true})
	_, err := s.TryRun("dnewslink", "-b", "bad%p", "-S", "0", "-h", "127.0.0.1", "-P", "119")
	if err == nil {
		t.Fatal("dnewslink accepted an unsupported batch-template conversion")
	}
}

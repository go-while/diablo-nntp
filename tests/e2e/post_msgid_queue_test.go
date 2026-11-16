package e2e

import (
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	testkit "github.com/go-while/nntp-testkit"
	"github.com/go-while/nntp-testkit/client"
	"github.com/go-while/nntp-testkit/fakeserver"
)

// TestPostQueuedKeepsMessageID: two clients post through the one connection to the outgoing
// server, the second while the first article is still unanswered, so its post is queued behind
// the first. Each client gets "240 <its Message-ID> Article posted". Before, when the first post
// was answered, NNPostResponse2() freed the Message-ID of the request that was then at the head
// of the queue, the second client's, and that client got "240  Article posted" without it.
func TestPostQueuedKeepsMessageID(t *testing.T) {
	release := make(chan struct{})
	var once sync.Once
	free := func() { once.Do(func() { close(release) }) }
	t.Cleanup(free)

	var n atomic.Int32
	var mu sync.Mutex
	conns := map[int]bool{}
	post := fakeserver.New(t, fakeserver.Options{Handler: fakeserver.LineHandler(
		func(c *fakeserver.Conn, line string) ([]string, bool) {
			f := strings.Fields(line)
			if len(f) == 0 {
				return nil, false
			}
			switch strings.ToUpper(f[0]) {
			case "IHAVE":
				mu.Lock()
				conns[c.ID()] = true
				mu.Unlock()
				if c.WriteLines("335 go") != nil {
					return nil, true
				}
				if _, err := c.ReadDotBlock(); err != nil {
					return nil, true
				}
				if n.Add(1) == 1 {
					// the first article stays unanswered until the second post is queued
					select {
					case <-release:
					case <-time.After(60 * time.Second):
					}
				}
				return []string{"235 ok"}, false
			case "QUIT":
				return []string{"205 bye"}, true
			}
			return []string{"500 unexpected"}, false
		})})
	_, spool := spoolStore(t)
	s := NewReaderSite(t, ReaderOptions{Spool: spool, Post: post})
	t.Cleanup(func() {
		if t.Failed() {
			t.Logf("general.log:\n%s", generalLog(s))
		}
	})

	send := func(id string) *client.Conn {
		c := s.Reader(t)
		c.ExpectCmd(340, "POST")
		c.MustSend(client.Stuff([]string{
			"From: E2E Poster <poster@example.invalid>",
			"Newsgroups: e2e.test",
			"Subject: queued post",
			"Message-ID: " + id,
			"",
			"body",
		}))
		return c
	}
	idA, idB := msgID(t), msgID(t)
	a := send(idA)
	defer a.Close()
	testkit.WaitFor(t, 20*time.Second, "the first article at the outgoing server", func() bool { return n.Load() == 1 })
	b := send(idB)
	defer b.Close()
	// dreaderd logs "post ok" when it hands the article to the queue of the outgoing server
	testkit.WaitFor(t, 20*time.Second, "the second post in dreaderd's queue", func() bool {
		return strings.Contains(generalLog(s), "post ok "+idB)
	})
	free()

	for _, p := range []struct {
		name string
		c    *client.Conn
		id   string
	}{{"first", a, idA}, {"second", b, idB}} {
		p.c.Deadline = 20 * time.Second
		code, line, err := p.c.ReadStatus()
		if err != nil || code != 240 {
			t.Fatalf("the %s post: reply %d %q %v, want 240", p.name, code, line, err)
		}
		if !strings.Contains(line, p.id) {
			t.Errorf("the %s post: reply %q has no Message-ID, want %s in it", p.name, line, p.id)
		}
	}
	if got := n.Load(); got != 2 {
		t.Errorf("the outgoing server got %d articles, want 2", got)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(conns) != 1 {
		t.Errorf("the posts came over %d connections, want 1: the second was not queued behind the first", len(conns))
	}
}

package e2e

import (
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	testkit "github.com/go-while/nntp-testkit"
	"github.com/go-while/nntp-testkit/fakeserver"
)

// heldPostServer returns a fake post server that holds its 335 to the first IHAVE for hold
// and then takes every article, and a function returning the articles it got (wire lines).
func heldPostServer(t *testing.T, hold time.Duration) (*fakeserver.Server, func() [][]string) {
	t.Helper()
	var mu sync.Mutex
	var arts [][]string
	var offers atomic.Int32
	post := fakeserver.New(t, fakeserver.Options{Deadline: 90 * time.Second, Handler: fakeserver.PeerHandler(&fakeserver.PeerOptions{
		OnArticle: func(id string, wire []string) {
			mu.Lock()
			defer mu.Unlock()
			arts = append(arts, wire)
		},
		BeforeReply: func(_ *fakeserver.Conn, cmd, _, status string) {
			if cmd == "IHAVE" && strings.HasPrefix(status, "335") && offers.Add(1) == 1 {
				time.Sleep(hold)
			}
		},
	})})
	return post, func() [][]string {
		mu.Lock()
		defer mu.Unlock()
		return append([][]string(nil), arts...)
	}
}

// TestPostPipelinedCommandWhileQueued (deep review r2 H5, D-228): after the "." of a POST,
// dreaderd handed the article to the post server but left the client in the article state
// (co_Func stayed NNAcceptArticle). What the client sent before its 240 was read as more
// lines of the article on its way to the post server: a command was posted as body text
// and never answered, a second "." ran the post code again on the buffer in flight (a
// second reply, and the buffer freed), and a fetch command made a second server request
// for the same client. The client now waits for the post server's answer, as a fetch does,
// and its next lines are read as commands after the 240.
//
// NNServerRequest() takes the client's descriptor out of RFds, so the lines were read on
// the next pass over all descriptors (forceallcheck, every 30 s). The post server here holds
// its 335 for 35 s, the client sends ".", GROUP and BODY 1 s after the post's ".", in one
// write, and a second client keeps dreaderd's loop turning, so one such pass falls while
// the post is in flight. Each command gets its own reply, in order, and the post server
// gets the article once, as posted.
func TestPostPipelinedCommandWhileQueued(t *testing.T) {
	st, spool := spoolStore(t)
	post, got := heldPostServer(t, 35*time.Second)
	s := NewReaderSite(t, ReaderOptions{Spool: spool, Post: post})
	t.Cleanup(func() {
		if t.Failed() {
			t.Logf("general.log:\n%s", generalLog(s))
		}
	})
	stored := msgID(t)
	feedReader(t, s, st, "e2e.test", newArticle(stored, "e2e.test", "to read", "stored body"))

	// dreaderd looks at the clock only every 10 selects (2 s each when idle), so on an idle
	// process the 30 s pass can be up to 50 s apart; a second client keeps it turning
	busy := s.Reader(t)
	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			select {
			case <-stop:
				return
			case <-time.After(200 * time.Millisecond):
			}
			if _, _, err := busy.TryCmd("GROUP e2e.test"); err != nil {
				return
			}
		}
	}()
	defer func() { close(stop); <-done }()

	id := msgID(t)
	body := []string{"first body line", "last body line"}
	c := s.Reader(t)
	defer c.Close()
	c.ExpectCmd(340, "POST")
	c.MustSend("From: a <a@example.invalid>\r\nNewsgroups: e2e.test\r\nSubject: pipelined\r\nMessage-ID: " + id +
		"\r\n\r\n" + strings.Join(body, "\r\n") + "\r\n.\r\n")
	// in its own write while the post waits for the 335: a second ".", a command, a fetch
	time.Sleep(time.Second)
	c.MustSend(".\r\nGROUP e2e.test\r\nBODY " + stored + "\r\n")

	c.Deadline = 60 * time.Second
	if code, line, err := c.ReadStatus(); code != 240 {
		t.Errorf("POST reply %d %q (%v), want 240", code, line, err)
	}
	c.Deadline = 10 * time.Second
	if code, line, err := c.ReadStatus(); code != 500 {
		t.Errorf(`reply to the "." sent after the post's dot: %d %q (%v), want 500`, code, line, err)
	}
	if code, line, err := c.ReadStatus(); code != 211 {
		t.Errorf("reply to the GROUP sent after the post's dot: %d %q (%v), want 211", code, line, err)
	}
	if code, line, err := c.ReadStatus(); code != 222 {
		t.Errorf("reply to the BODY sent after the post's dot: %d %q (%v), want 222", code, line, err)
	} else if b, err := c.TryReadDotLines(); err != nil || strings.Join(b, "|") != "stored body" {
		t.Errorf("BODY %s: %q (%v), want [stored body]", stored, b, err)
	}
	if err := c.ExpectNoPending(2 * time.Second); err != nil {
		t.Errorf("after the BODY: %v", err)
	}

	var arts [][]string
	testkit.WaitFor(t, 15*time.Second, "the post server to get the article", func() bool {
		arts = got()
		return len(arts) > 0
	})
	// a second post would come after the first one's 235
	time.Sleep(time.Second)
	if arts = got(); len(arts) != 1 {
		t.Errorf("the post server got %d articles, want 1: %q", len(arts), arts)
	}
	if _, b := splitArticle(arts[0]); strings.Join(b, "|") != strings.Join(body, "|") {
		t.Errorf("body at the post server %q, want %q", b, body)
	}
}

// TestPostClientGoneWhileQueued: a client that closes its connection right after the "."
// of a POST (and a pipelined GROUP), while the post server still holds its 335, does not stop
// the reader process or the post connection: the next client's POST and GROUP are answered
// and its article reaches the post server.
func TestPostClientGoneWhileQueued(t *testing.T) {
	_, spool := spoolStore(t)
	post, got := heldPostServer(t, 3*time.Second)
	s := NewReaderSite(t, ReaderOptions{Spool: spool, Post: post})
	t.Cleanup(func() {
		if t.Failed() {
			t.Logf("general.log:\n%s", generalLog(s))
		}
	})
	head := func(id string) string {
		return "From: a <a@example.invalid>\r\nNewsgroups: e2e.test\r\nSubject: gone\r\nMessage-ID: " + id + "\r\n\r\n"
	}

	// its article is posted or dropped, depending on when dreaderd sees the close
	c := s.Reader(t)
	c.ExpectCmd(340, "POST")
	c.MustSend(head(msgID(t)) + "body\r\n.\r\nGROUP e2e.test\r\n")
	c.Close()

	next := msgID(t)
	d := s.Reader(t)
	defer d.Close()
	d.ExpectCmd(340, "POST")
	d.MustSend(head(next) + "body\r\n.\r\n")
	d.Deadline = 30 * time.Second
	if code, line, err := d.ReadStatus(); code != 240 {
		t.Errorf("POST after the closed one: %d %q (%v), want 240", code, line, err)
	}
	d.Deadline = 10 * time.Second
	d.ExpectCmd(211, "GROUP e2e.test")

	testkit.WaitFor(t, 15*time.Second, "the next client's article at the post server", func() bool {
		for _, a := range got() {
			if h, _ := splitArticle(a); headerValue(h, "Message-ID") == next {
				return true
			}
		}
		return false
	})
}

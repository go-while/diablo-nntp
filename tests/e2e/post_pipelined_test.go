package e2e

import (
	"fmt"
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

// keepBusy keeps dreaderd's loop turning with a client that sends GROUP every 200 ms:
// dreaderd looks at the clock only every 10 selects (2 s each when idle), so on an idle
// process the pass over all descriptors, due every 30 s, can be up to 50 s apart. The returned
// function stops it and fails t if a GROUP of it failed meanwhile.
func keepBusy(t *testing.T, s *Site) func() {
	t.Helper()
	busy := s.Reader(t)
	stop := make(chan struct{})
	done := make(chan struct{})
	var berr error
	go func() {
		defer close(done)
		for {
			select {
			case <-stop:
				return
			case <-time.After(200 * time.Millisecond):
			}
			if code, line, err := busy.TryCmd("GROUP e2e.test"); err != nil || code != 211 {
				berr = fmt.Errorf("GROUP on the busy client: %d %q (%v)", code, line, err)
				return
			}
		}
	}()
	return func() {
		close(stop)
		<-done
		if berr != nil {
			t.Errorf("%v", berr)
		}
	}
}

// TestPostPipelinedCommandWhileQueued (D-228): after the "." of a POST,
// dreaderd handed the article to the post server but left the client in the article state
// (co_Func stayed NNAcceptArticle). What the client sent before its 240 went to the article
// code: a command was posted as body text and not answered, and a second "." ran the post
// code again on the buffer in flight (a second reply, and the buffer freed) and put the
// client back to commands while its post was still in flight, so a fetch command after it
// made a second server request for the same client. The client now waits for the post
// server's answer, as a fetch does,
// and its next lines are read as commands after the 240.
//
// NNServerRequest() takes the client's descriptor out of RFds, so the lines were read on
// the next pass over all descriptors (forceallcheck, every 30 s). The post server here holds
// its 335 for 45 s, the client sends ".", GROUP and BODY 1 s after the post's ".", in one
// write, and a second client keeps dreaderd's loop turning, so one such pass falls while
// the post is in flight. Each command gets its own reply, in order, and the post server
// gets the article once, as posted.
func TestPostPipelinedCommandWhileQueued(t *testing.T) {
	st, spool := spoolStore(t)
	post, got := heldPostServer(t, 45*time.Second)
	s := NewReaderSite(t, ReaderOptions{Spool: spool, Post: post})
	t.Cleanup(func() {
		if t.Failed() {
			t.Logf("general.log:\n%s", generalLog(s))
		}
	})
	stored := msgID(t)
	feedReader(t, s, st, "e2e.test", newArticle(stored, "e2e.test", "to read", "stored body"))

	defer keepBusy(t, s)()

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

// TestPostClientGoneWhileQueued: a client whose connection fails (here: it is reset) while
// its POST waits for the post server is let go at the next pass over all descriptors, before
// the post server answers. A waiting client is not read (NNWaitThread), so without a look at
// its socket it was held, with its descriptor and its article, until the post server
// answered, however long that took. A clean close or half-close is not taken for gone: such
// a client keeps its connection and its post until the answer (TestPostClientCloseKeepsPost).
// The post server holds its 335 for 48 s; dreaderd must report the client closed within 44 s
// of the reset (the pass comes every 30 s), while a busy client and a new client keep being
// served; the new client's POST is answered after the held one.
func TestPostClientGoneWhileQueued(t *testing.T) {
	_, spool := spoolStore(t)
	post, got := heldPostServer(t, 48*time.Second)
	s := NewReaderSite(t, ReaderOptions{Spool: spool, Post: post})
	t.Cleanup(func() {
		if t.Failed() {
			t.Logf("general.log:\n%s", generalLog(s))
		}
	})
	defer keepBusy(t, s)()
	head := func(id string) string {
		return "From: a <a@example.invalid>\r\nNewsgroups: e2e.test\r\nSubject: gone\r\nMessage-ID: " + id + "\r\n\r\n"
	}

	c := s.Reader(t)
	c.ExpectCmd(340, "POST")
	c.MustSend(head(msgID(t)) + "body\r\n.\r\n")
	// dreaderd logs "closed from 127.0.0.1 (127.0.0.1:<port>) ..." when it lets a client go
	tag := "(" + c.Conn.LocalAddr().String() + ")"
	// give dreaderd the "." before the reset
	time.Sleep(time.Second)
	resetClose(t, c)
	closed := func() bool {
		for _, l := range strings.Split(generalLog(s), "\n") {
			if strings.Contains(l, "closed from") && strings.Contains(l, tag) {
				return true
			}
		}
		return false
	}
	testkit.WaitFor(t, 44*time.Second, "dreaderd to let the closed posting client "+tag+" go", closed)
	if n := len(got()); n != 0 {
		t.Errorf("the post server had already answered (%d articles): the client was let go only then", n)
	}

	// the reader process still serves new clients while the post connection waits
	next := msgID(t)
	d := s.Reader(t)
	defer d.Close()
	d.ExpectCmd(211, "GROUP e2e.test")
	d.ExpectCmd(340, "POST")
	d.MustSend(head(next) + "body\r\n.\r\n")
	d.Deadline = 60 * time.Second
	if code, line, err := d.ReadStatus(); code != 240 {
		t.Errorf("POST after the closed one: %d %q (%v), want 240", code, line, err)
	}
	testkit.WaitFor(t, 15*time.Second, "the next client's article at the post server", func() bool {
		for _, a := range got() {
			if h, _ := splitArticle(a); headerValue(h, "Message-ID") == next {
				return true
			}
		}
		return false
	})
}

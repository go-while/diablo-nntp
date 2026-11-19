package e2e

import (
	"errors"
	"fmt"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	testkit "github.com/go-while/nntp-testkit"
	"github.com/go-while/nntp-testkit/client"
	"github.com/go-while/nntp-testkit/fakeserver"
)

// postSink is a fake outgoing ('o') server for dreaderd: it answers the commands dreaderd
// primes a server connection with by 500, and IHAVE by 335; then onIHave decides what
// happens to the article. Articles read up to their "." line are kept (wire lines, still
// dot-stuffed) and answered 235.
type postSink struct {
	mu     sync.Mutex
	arts   [][]string
	offers atomic.Int32
	// onIHave runs after the 335 of the n-th IHAVE (from 1, over all connections) and
	// returns the article lines it read itself, kept in front of the rest; false: close the
	// connection without reading the article to its end.
	onIHave func(c *fakeserver.Conn, n int) ([]string, bool)
}

func (p *postSink) handler(c *fakeserver.Conn) {
	for {
		line, err := c.ReadLine()
		if err != nil {
			return
		}
		f := strings.Fields(line)
		if len(f) == 0 {
			_ = c.WriteLines("500 unknown command")
			continue
		}
		switch strings.ToUpper(f[0]) {
		case "IHAVE":
			if c.WriteLines("335 Send it") != nil {
				return
			}
			var pre []string
			if p.onIHave != nil {
				var ok bool
				if pre, ok = p.onIHave(c, int(p.offers.Add(1))); !ok {
					return
				}
			}
			rest, err := c.ReadDotBlock()
			if err != nil {
				return
			}
			block := append(pre, rest...)
			p.mu.Lock()
			p.arts = append(p.arts, block)
			p.mu.Unlock()
			if c.WriteLines("235 Article transferred OK") != nil {
				return
			}
		case "QUIT":
			_ = c.WriteLines("205 bye")
			return
		default:
			_ = c.WriteLines("500 unknown command")
		}
	}
}

func (p *postSink) got() [][]string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([][]string(nil), p.arts...)
}

func newPostSink(t *testing.T, onIHave func(c *fakeserver.Conn, n int) ([]string, bool)) (*postSink, *fakeserver.Server) {
	t.Helper()
	p := &postSink{onIHave: onIHave}
	return p, fakeserver.New(t, fakeserver.Options{Deadline: 120 * time.Second, Handler: p.handler})
}

// postBody returns n body lines of about 70 octets, the 5th a line with a leading dot
// (sent and passed on dot-stuffed), as the post server must get them (wire, still stuffed).
func postBody(n int) []string {
	body := make([]string, n)
	for i := range body {
		body[i] = fmt.Sprintf("body line %06d %s", i, strings.Repeat("x", 54))
	}
	if n > 4 {
		body[4] = "..a line with a leading dot"
	}
	return body
}

func requeuePostHead(id, subject string) string {
	return "From: a <a@example.invalid>\r\nNewsgroups: e2e.test\r\nSubject: " + subject +
		"\r\nMessage-ID: " + id + "\r\n\r\n"
}

// checkWhole fails t unless the post server got exactly one article, with Message-ID id and
// the body as posted.
func checkWhole(t *testing.T, arts [][]string, id string, body []string) {
	t.Helper()
	if len(arts) != 1 {
		t.Errorf("the post server got %d articles, want 1", len(arts))
	}
	for i, a := range arts {
		h, b := splitArticle(a)
		if got := headerValue(h, "Message-ID"); got != id {
			t.Errorf("article %d at the post server: Message-ID %q, want %q (%d lines, first %q, last %q)",
				i, got, id, len(a), headOfLines(a), tailOfLines(a))
			continue
		}
		if strings.Join(b, "\n") != strings.Join(body, "\n") {
			t.Errorf("article %d at the post server: %d body lines, want %d (first %.120q, last %.120q)",
				i, len(b), len(body), headOfLines(b), tailOfLines(b))
		}
	}
}

func headOfLines(l []string) string {
	if len(l) == 0 {
		return ""
	}
	return l[0]
}

func tailOfLines(l []string) string {
	if len(l) == 0 {
		return ""
	}
	return l[len(l)-1]
}

// TestPostRequeueWholeArticle: a post server that answers 335 to IHAVE and then goes
// away before its 235 (closes after reading none or a few lines of the article) gets the
// post requeued, and dreaderd sends it to the next server connection. That server got only
// what was left of the article: dreaderd read the article out of the client's buffer as it
// sent it, so the next server got the rest of it, or only the "." line (an empty article),
// and answered 235 to that. Now the article stays until the post is answered: the next
// connection (dreaderd reconnects to the same server here) gets the whole article once, and
// the client gets its 240.
func TestPostRequeueWholeArticle(t *testing.T) {
	for _, tc := range []struct {
		name  string
		lines int // article lines the first connection reads before it closes
	}{
		{"close right after 335", 0},
		{"close after three lines", 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, spool := spoolStore(t)
			sink, post := newPostSink(t, func(c *fakeserver.Conn, n int) ([]string, bool) {
				if n != 1 {
					return nil, true
				}
				for i := 0; i < tc.lines; i++ {
					if _, err := c.ReadLine(); err != nil {
						break
					}
				}
				return nil, false
			})
			s := NewReaderSite(t, ReaderOptions{Spool: spool, Post: post})
			t.Cleanup(func() {
				if t.Failed() {
					t.Logf("general.log:\n%s", generalLog(s))
				}
			})
			// the reconnect comes with the check of the server list every 5 s of dreaderd's clock
			defer keepBusy(t, s)()

			id := msgID(t)
			body := postBody(40)
			c := s.Reader(t)
			defer c.Close()
			c.ExpectCmd(340, "POST")
			c.MustSend(requeuePostHead(id, "requeued") + strings.Join(body, "\r\n") + "\r\n.\r\n")
			c.Deadline = 60 * time.Second
			if code, line, err := c.ReadStatus(); code != 240 {
				t.Errorf("POST reply %d %q (%v), want 240", code, line, err)
			}
			if n := sink.offers.Load(); n < 2 {
				t.Errorf("the post server got %d IHAVE, want 2 (the first connection closed after its 335)", n)
			}
			// a second copy would come right after the first one's 235
			time.Sleep(time.Second)
			checkWhole(t, sink.got(), id, body)
		})
	}
}

// TestPostClientTimeoutWholeArticle: a client whose idle timeout ends it while its
// article is on its way to a slow post server. dreaderd sent the article a piece at a time
// as the post server took it; when the client was gone it sent the "." line after the
// pieces already sent, so the post server stored a cut article (and its 235 went nowhere).
// Now the whole article is queued for the post server when it answers 335, so the post
// server gets all of it whether the client stays or not.
//
// The article is 1 MiB and the post server reads a few lines, then nothing until the client
// is gone (readerdef idletimeout 5; the waiting client is let go at the next pass over all
// descriptors, every 30 s, which a busy client keeps coming); dreaderd's send buffer for the
// post server is 16 KiB (txbufsize), so with what the post server's socket takes most of the
// article is still in dreaderd then.
func TestPostClientTimeoutWholeArticle(t *testing.T) {
	_, spool := spoolStore(t)
	release := make(chan struct{})
	var once sync.Once
	free := func() { once.Do(func() { close(release) }) }
	t.Cleanup(free)
	sink, post := newPostSink(t, func(c *fakeserver.Conn, n int) ([]string, bool) {
		var pre []string
		if n == 1 {
			for i := 0; i < 5; i++ {
				l, err := c.ReadLine()
				if err != nil {
					return nil, false
				}
				pre = append(pre, l)
			}
			select {
			case <-release:
			case <-time.After(80 * time.Second):
			}
		}
		return pre, true
	})
	hosts := "127.0.0.1\ts1p" + fmt.Sprint(spool.Port()) + "\n" +
		"127.0.0.1\to1p" + fmt.Sprint(post.Port()) + " txbufsize=16384\n"
	s := NewReaderSite(t, ReaderOptions{
		Spool:  spool,
		Post:   post,
		Access: accessWith("idletimeout 5"),
		Config: map[string]string{"readermaxartsize": "4m"},
		Files:  map[string]string{"dserver.hosts": hosts},
	})
	t.Cleanup(func() {
		if t.Failed() {
			t.Logf("general.log:\n%s", generalLog(s))
		}
	})
	defer keepBusy(t, s)()

	id := msgID(t)
	body := postBody(15000) // about 1 MiB
	c := s.Reader(t)
	defer c.Close()
	c.ExpectCmd(340, "POST")
	c.Deadline = 60 * time.Second
	c.MustSend(requeuePostHead(id, "timed out") + strings.Join(body, "\r\n") + "\r\n.\r\n")

	// the client is let go by its idle timeout: a "400 ... Idle timeout." line, or EOF or a
	// reset; a read that only hits its deadline means the client was not let go, and the
	// article would then go through with the client still there (not the case this test is for)
	c.Deadline = 75 * time.Second
	code, line, err := c.ReadStatus()
	t.Logf("client: %d %q (%v)", code, line, err)
	switch {
	case err == nil && code == 400:
		if l, err := c.ReadLine(); err == nil {
			t.Fatalf("after %q the client got %q, want the connection closed", line, l)
		} else if isTimeout(err) {
			t.Fatalf("after %q the connection stayed open: %v", line, err)
		}
	case err == nil:
		t.Fatalf("POST reply %d %q while the post server held the article, want the idle timeout's 400 or a closed connection", code, line)
	case isTimeout(err):
		t.Fatalf("the client was not let go by its idle timeout within 75 s: %v", err)
	}
	if !strings.Contains(generalLog(s), "timeout idle") {
		t.Fatalf("no \"timeout idle\" line in general.log: the client was not ended by its idle timeout")
	}
	free()

	testkit.WaitFor(t, 60*time.Second, "the post server to get the article", func() bool {
		return len(sink.got()) > 0
	})
	time.Sleep(time.Second)
	checkWhole(t, sink.got(), id, body)
}

// waitLetGo waits until dreaderd logs that it let the client with local address tag go
// ("closed from 127.0.0.1 (<tag>) ...", letGo).
func waitLetGo(t *testing.T, s *Site, tag string, d time.Duration) {
	t.Helper()
	testkit.WaitFor(t, d, "dreaderd to let the posting client "+tag+" go", func() bool {
		return letGo(s, tag)
	})
}

// onlyArticles fails t unless the post server got exactly the articles with these
// Message-IDs, in this order.
func onlyArticles(t *testing.T, arts [][]string, ids ...string) {
	t.Helper()
	var got []string
	for _, a := range arts {
		h, _ := splitArticle(a)
		got = append(got, fmt.Sprintf("%q (%d lines)", headerValue(h, "Message-ID"), len(a)))
	}
	want := make([]string, len(ids))
	ok := len(arts) == len(ids)
	for i, id := range ids {
		want[i] = fmt.Sprintf("%q", id)
		if ok {
			h, _ := splitArticle(arts[i])
			ok = headerValue(h, "Message-ID") == id
		}
	}
	if !ok {
		t.Errorf("the post server got %v, want only %v", got, want)
	}
}

// TestPostClientGoneAfterIhave (no article lines sent yet): a client that goes
// away after dreaderd sent the IHAVE for its post, before the 335 (here: its connection is
// reset while the post server holds its 335 for 48 s, and dreaderd lets it go at the next
// pass over all descriptors; idle and session timeouts end a client the same way). Its article is gone with it; dreaderd answered the 335 with the "." line
// alone, an empty article, which the post server here stores (a lax server could). Now
// dreaderd closes the post server connection instead, so nothing is stored; the next
// client's POST goes over a new connection and gets 240, and it is the only article the
// post server gets.
func TestPostClientGoneAfterIhave(t *testing.T) {
	_, spool := spoolStore(t)
	post, got := heldPostServer(t, 48*time.Second)
	s := NewReaderSite(t, ReaderOptions{Spool: spool, Post: post})
	t.Cleanup(func() {
		if t.Failed() {
			t.Logf("general.log:\n%s", generalLog(s))
		}
	})
	defer keepBusy(t, s)()

	gone := msgID(t)
	c := s.Reader(t)
	c.ExpectCmd(340, "POST")
	c.MustSend(requeuePostHead(gone, "gone after ihave") + "body\r\n.\r\n")
	tag := "(" + c.Conn.LocalAddr().String() + ")"
	time.Sleep(time.Second)
	resetClose(t, c)
	waitLetGo(t, s, tag, 44*time.Second)
	if n := len(got()); n != 0 {
		t.Fatalf("the post server answered (%d articles) before the client was let go: the test did not hold it", n)
	}

	next := msgID(t)
	d := s.Reader(t)
	defer d.Close()
	d.ExpectCmd(340, "POST")
	d.MustSend(requeuePostHead(next, "after the gone one") + "body\r\n.\r\n")
	d.Deadline = 60 * time.Second
	if code, line, err := d.ReadStatus(); code != 240 {
		t.Errorf("POST after the gone one: %d %q (%v), want 240", code, line, err)
	}
	time.Sleep(time.Second)
	onlyArticles(t, got(), next)
}

// TestPostQueuedClientGoneNotOffered (a post queued behind another): a client whose
// post waits behind another client's post on the post server connection, and whose
// connection is reset there (dreaderd lets it go at the next pass over all descriptors). When the post
// before it was answered, dreaderd offered the gone client's post anyway (IHAVE) and
// answered the 335 with the "." line alone: an empty article, which the post server here
// stores. Now a post whose client is gone is dropped before its IHAVE: the post server gets
// one IHAVE and the first client's article only, and the first client gets its 240.
func TestPostQueuedClientGoneNotOffered(t *testing.T) {
	_, spool := spoolStore(t)
	post, got := heldPostServer(t, 48*time.Second)
	s := NewReaderSite(t, ReaderOptions{Spool: spool, Post: post})
	t.Cleanup(func() {
		if t.Failed() {
			t.Logf("general.log:\n%s", generalLog(s))
		}
	})
	defer keepBusy(t, s)()

	first := msgID(t)
	a := s.Reader(t)
	defer a.Close()
	a.ExpectCmd(340, "POST")
	a.MustSend(requeuePostHead(first, "first") + "body\r\n.\r\n")
	time.Sleep(time.Second)

	gone := msgID(t)
	b := s.Reader(t)
	b.ExpectCmd(340, "POST")
	b.MustSend(requeuePostHead(gone, "queued and gone") + "body\r\n.\r\n")
	tag := "(" + b.Conn.LocalAddr().String() + ")"
	time.Sleep(time.Second)
	resetClose(t, b)
	waitLetGo(t, s, tag, 44*time.Second)
	if n := len(got()); n != 0 {
		t.Fatalf("the post server answered (%d articles) before the client was let go: the test did not hold it", n)
	}

	a.Deadline = 60 * time.Second
	if code, line, err := a.ReadStatus(); code != 240 {
		t.Errorf("POST of the first client: %d %q (%v), want 240", code, line, err)
	}
	// the gone client's post would be offered right after the first one's 235
	time.Sleep(3 * time.Second)
	if n := post.Count("IHAVE"); n != 1 {
		t.Errorf("the post server got %d IHAVE, want 1 (none for the gone client's post)", n)
	}
	onlyArticles(t, got(), first)
}

// resetClose closes c with SO_LINGER 0: the peer gets a reset (a socket error), not a clean
// close.
func resetClose(t *testing.T, c *client.Conn) {
	t.Helper()
	tc, ok := c.Conn.(*net.TCPConn)
	if !ok {
		t.Fatalf("client connection is %T, not TCP", c.Conn)
	}
	if err := tc.SetLinger(0); err != nil {
		t.Fatalf("SO_LINGER 0: %v", err)
	}
	c.Close()
}

// letGo reports whether dreaderd logged that it let the client with local address tag go.
func letGo(s *Site, tag string) bool {
	for _, l := range strings.Split(generalLog(s), "\n") {
		if strings.Contains(l, "closed from") && strings.Contains(l, tag) {
			return true
		}
	}
	return false
}

// TestPostClientCloseKeepsPost: a client that sends a complete POST and then closes its
// connection cleanly, or half-closes it (shutdown of its write side, no QUIT), while the
// post server holds its 335 for 60 s (the check below at 44 s after the close, past one pass,
// leaves about 15 s before the 335). The look at a waiting posting client's socket (D-228)
// took the EOF for a client gone and let it go at the next pass over all descriptors; its
// post was then dropped (the post server got a "." alone). A clean close or half-close is no
// longer taken for gone, only a socket error is: the client keeps its connection until the
// answer (not let go 44 s after the close, past one pass), the post server gets the article
// once, whole, a half-closed client still reads its 240, and the reader process serves other
// clients meanwhile.
func TestPostClientCloseKeepsPost(t *testing.T) {
	for _, tc := range []struct {
		name string
		half bool
	}{
		{"half-close", true},
		{"close", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, spool := spoolStore(t)
			post, got := heldPostServer(t, 60*time.Second)
			s := NewReaderSite(t, ReaderOptions{Spool: spool, Post: post})
			t.Cleanup(func() {
				if t.Failed() {
					t.Logf("general.log:\n%s", generalLog(s))
				}
			})
			defer keepBusy(t, s)()

			id := msgID(t)
			body := postBody(40)
			c := s.Reader(t)
			defer c.Close()
			c.ExpectCmd(340, "POST")
			c.MustSend(requeuePostHead(id, tc.name) + strings.Join(body, "\r\n") + "\r\n.\r\n")
			tag := "(" + c.Conn.LocalAddr().String() + ")"
			time.Sleep(time.Second)
			closed := time.Now()
			if tc.half {
				if err := c.Conn.(*net.TCPConn).CloseWrite(); err != nil {
					t.Fatalf("half-close: %v", err)
				}
			} else {
				c.Close()
			}

			// another client is served while the post waits
			time.Sleep(5 * time.Second)
			o := s.Reader(t)
			o.ExpectCmd(211, "GROUP e2e.test")
			o.Close()

			// one pass over all descriptors (every 30 s) falls in this time
			time.Sleep(time.Until(closed.Add(44 * time.Second)))
			if letGo(s, tag) {
				t.Errorf("the client %s was let go before its post was answered", tag)
			}
			if n := len(got()); n != 0 {
				t.Fatalf("the post server answered (%d articles) within 44 s: the test did not hold it", n)
			}
			if tc.half {
				c.Deadline = 45 * time.Second
				if code, line, err := c.ReadStatus(); code != 240 {
					t.Errorf("half-closed client: POST reply %d %q (%v), want 240", code, line, err)
				}
			}
			testkit.WaitFor(t, 30*time.Second, "the post server to get the article", func() bool {
				return len(got()) > 0
			})
			time.Sleep(time.Second)
			checkWhole(t, got(), id, body)
			testkit.WaitFor(t, 15*time.Second, "dreaderd to close the client "+tag+" after the answer", func() bool {
				return letGo(s, tag)
			})
		})
	}
}

// isTimeout reports whether err is a read deadline that passed (not EOF or a reset).
func isTimeout(err error) bool {
	var ne net.Error
	return errors.As(err, &ne) && ne.Timeout()
}

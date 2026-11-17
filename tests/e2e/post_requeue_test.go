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

// TestPostRequeueWholeArticle (F1): a post server that answers 335 to IHAVE and then goes
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

// TestPostClientTimeoutWholeArticle (F2): a client whose idle timeout ends it while its
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

	// the client is let go by its idle timeout (a "400 ... Idle timeout." line, or only EOF)
	c.Deadline = 75 * time.Second
	code, line, err := c.ReadStatus()
	if code == 240 {
		t.Fatalf("POST answered 240 while the post server held the article: the test did not hold it")
	}
	t.Logf("client: %d %q (%v)", code, line, err)
	if err == nil {
		if l, err := c.ReadLine(); err == nil {
			t.Errorf("after %q the client got %q, want the connection closed", line, l)
		}
	}
	free()

	testkit.WaitFor(t, 60*time.Second, "the post server to get the article", func() bool {
		return len(sink.got()) > 0
	})
	time.Sleep(time.Second)
	checkWhole(t, sink.got(), id, body)
}

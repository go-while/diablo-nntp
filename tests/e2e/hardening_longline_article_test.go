package e2e

// dreaderd: an article with a line over the limit is refused, the connection stays open (a
// feeder that is dropped offers the article again). A command line over the limit still
// closes the connection, after a 500.

import (
	"github.com/go-while/nntp-testkit/client"
	"net"
	"strings"
	"testing"

	"github.com/go-while/nntp-testkit/fakeserver"
)

// longLine is a line of 2 MiB, twice the limit.
var longLine = strings.Repeat("L", 2*clientMaxLine)

// TestReaderIHaveLongLine: IHAVE of an article with a 2 MiB body line gets 437 and the
// connection stays open: the next IHAVE is taken (235).
func TestReaderIHaveLongLine(t *testing.T) {
	st, spool := spoolStore(t)
	s := NewReaderSite(t, ReaderOptions{Spool: spool})
	p := s.dialFrom(t, PeerIP)

	id := msgID(t)
	p.ExpectCmd(335, "IHAVE %s", id)
	// text around the long line is read normally, as is the rest after its end
	p.MustSend(client.Stuff(newArticle(id, "e2e.test", "long line", "before", longLine, "after")))
	p.ExpectStatus(437, "IHAVE article with a long line")

	id2 := msgID(t)
	art := newArticle(id2, "e2e.test", "normal", "hello")
	head, body := splitArticle(art)
	st.AddArticle("e2e.test", fakeserver.Article{MessageID: id2, Headers: head, Body: body})
	p.ExpectCmd(335, "IHAVE %s", id2)
	p.MustSend(client.Stuff(art))
	p.ExpectStatus(235, "IHAVE normal article")
	p.ExpectCmd(205, "QUIT")
}

// TestReaderTakeThisLongLine: the same with TAKETHIS, answered 439.
func TestReaderTakeThisLongLine(t *testing.T) {
	_, spool := spoolStore(t)
	s := NewReaderSite(t, ReaderOptions{Spool: spool})
	p := s.dialFrom(t, PeerIP)

	id := msgID(t)
	p.MustSend("TAKETHIS " + id + "\r\n" + client.Stuff(newArticle(id, "e2e.test", "long line", longLine)))
	p.ExpectStatus(439, "TAKETHIS article with a long line")
	p.ExpectCmd(205, "QUIT")
}

// TestReaderPostLongLine: POST of an article with a 2 MiB body line gets 441 and the
// connection still works.
func TestReaderPostLongLine(t *testing.T) {
	_, spool := spoolStore(t)
	s := NewReaderSite(t, ReaderOptions{Spool: spool})
	c := s.Reader(t)

	c.ExpectCmd(340, "POST")
	c.MustSend(client.Stuff([]string{
		"From: E2E Poster <poster@example.invalid>",
		"Newsgroups: e2e.test",
		"Subject: long line",
		"Message-ID: " + msgID(t),
		"",
		"before",
		longLine,
		"after",
	}))
	c.ExpectStatus(441, "POST article with a long line")
	c.ExpectCmd(211, "GROUP e2e.test")
	c.ExpectCmd(205, "QUIT")
}

// TestReaderCommandLongLine: a command line of 2 MiB gets 500 and the connection is closed.
func TestReaderCommandLongLine(t *testing.T) {
	_, spool := spoolStore(t)
	s := NewReaderSite(t, ReaderOptions{Spool: spool})
	c := s.Reader(t)

	// A server is allowed to close as soon as it has seen the oversized line.
	// On systems with smaller TCP buffers this can interrupt the 2 MiB write.
	writeErr := c.Send("X" + longLine + "\r\n")
	if writeErr != nil {
		t.Logf("oversize command write: %v", writeErr)
	}
	code, line, err := c.ReadStatus()
	if err == nil {
		if code != 500 {
			t.Fatalf("command line over the limit: got %q, want 500", line)
		}
		if !strings.Contains(strings.ToLower(line), "line too long") {
			t.Fatalf("command line over the limit: got %q, want line too long", line)
		}
		if code, line, err := c.ReadStatus(); err == nil {
			t.Fatalf("after the 500: %d %q, want the connection closed", code, line)
		} else if ne, ok := err.(net.Error); ok && ne.Timeout() {
			t.Fatal("connection still open after the 500")
		}
		return
	}
	if ne, ok := err.(net.Error); ok && ne.Timeout() {
		t.Fatal("connection still open after the oversized command")
	}
	// A close/reset before the 500 is also valid: the limit was enforced
	// while the client was still writing.  Verify that it was the line limit,
	// rather than an unrelated reader failure.  Needs a FreeBSD/illumos CI run.
	if !strings.Contains(s.logOutput(), "MBReadLine line longer than") {
		t.Fatalf("oversized command closed without enforcing the line limit: %v", err)
	}
}

package e2e

// dreaderd: an article with a line over the limit is refused, the connection stays open (a
// feeder that is dropped offers the article again). A command line over the limit still
// closes the connection, after a 500.

import (
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
	expect(t, p, 335, "IHAVE %s", id)
	// text around the long line is read normally, as is the rest after its end
	send(t, p, wire(newArticle(id, "e2e.test", "long line", "before", longLine, "after")))
	status(t, p, 437, "IHAVE article with a long line")

	id2 := msgID(t)
	art := newArticle(id2, "e2e.test", "normal", "hello")
	head, body := splitArticle(art)
	st.AddArticle("e2e.test", fakeserver.Article{MessageID: id2, Headers: head, Body: body})
	expect(t, p, 335, "IHAVE %s", id2)
	send(t, p, wire(art))
	status(t, p, 235, "IHAVE normal article")
	expect(t, p, 205, "QUIT")
}

// TestReaderTakeThisLongLine: the same with TAKETHIS, answered 439.
func TestReaderTakeThisLongLine(t *testing.T) {
	_, spool := spoolStore(t)
	s := NewReaderSite(t, ReaderOptions{Spool: spool})
	p := s.dialFrom(t, PeerIP)

	id := msgID(t)
	send(t, p, "TAKETHIS "+id+"\r\n"+wire(newArticle(id, "e2e.test", "long line", longLine)))
	status(t, p, 439, "TAKETHIS article with a long line")
	expect(t, p, 205, "QUIT")
}

// TestReaderPostLongLine: POST of an article with a 2 MiB body line gets 441 and the
// connection still works.
func TestReaderPostLongLine(t *testing.T) {
	_, spool := spoolStore(t)
	s := NewReaderSite(t, ReaderOptions{Spool: spool})
	c := s.Reader(t)

	expect(t, c, 340, "POST")
	send(t, c, wire([]string{
		"From: E2E Poster <poster@example.invalid>",
		"Newsgroups: e2e.test",
		"Subject: long line",
		"Message-ID: " + msgID(t),
		"",
		"before",
		longLine,
		"after",
	}))
	status(t, c, 441, "POST article with a long line")
	expect(t, c, 211, "GROUP e2e.test")
	expect(t, c, 205, "QUIT")
}

// TestReaderCommandLongLine: a command line of 2 MiB gets 500 and the connection is closed.
func TestReaderCommandLongLine(t *testing.T) {
	_, spool := spoolStore(t)
	s := NewReaderSite(t, ReaderOptions{Spool: spool})
	c := s.Reader(t)

	send(t, c, "X"+longLine+"\r\n")
	status(t, c, 500, "command line over the limit")
	if code, line, err := c.ReadStatus(); err == nil {
		t.Fatalf("after the 500: %d %q, want the connection closed", code, line)
	} else if ne, ok := err.(net.Error); ok && ne.Timeout() {
		t.Fatal("connection still open after the 500")
	}
}

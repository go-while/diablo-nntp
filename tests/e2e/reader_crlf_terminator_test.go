package e2e

import (
	"strings"
	"testing"

	"github.com/go-while/nntp-testkit/fakeserver"
)

// TestReaderFeedBareLFDotIsData: the feed port of dreaderd takes an article by the same
// rule as diablo: only CRLF "." CRLF ends it (RFC 3977 3.1.1); a "." line the peer ended
// with a bare LF, and a "." that starts after a bare LF, are data, and one dot is cut only
// after CRLF. Before the fix NNAcceptArticle() ended the article at the first "." LF, kept
// the part before it, and read the rest of the body as commands (one 500 per line).
//
// The body has 8 lines: the overview's :lines counts them all, and the next command is
// answered as a command.
func TestReaderFeedBareLFDotIsData(t *testing.T) {
	st, spool := spoolStore(t)
	s := NewReaderSite(t, ReaderOptions{Spool: spool})
	id := msgID(t)
	art := newArticle(id, "e2e.test", "bare LF dot", "x")
	head, _ := splitArticle(art)
	raw := strings.Join(head, "\r\n") + "\r\n\r\n" +
		"quote.\r\n" +
		"text ended by a bare LF.\n" +
		".\n" + // data, not the end of the article
		".dotted after a bare LF\n" +
		"still the body\r\n" +
		"bare again\n" +
		".\r\n" + // CRLF after a bare LF: data
		"..stuffed after CRLF\r\n" +
		".\r\n" // the end
	st.AddArticle("e2e.test", fakeserver.Article{MessageID: id, Headers: head, Body: []string{"x"}})

	p := s.dialFrom(t, PeerIP)
	p.ExpectCmd(335, "IHAVE %s", id)
	p.MustSend(raw)
	p.ExpectStatus(235, "IHAVE "+id)
	// not read as the rest of the article
	p.ExpectCmd(205, "QUIT")

	c := s.Reader(t)
	waitGroupCount(t, c, "1")
	if got := overLines(t, c, "OVER 1-"); got["1"] != "8" {
		t.Errorf("OVER :lines %v, want 8 for the article's 8 body lines", got)
	}
	c.ExpectCmd(205, "QUIT")
}

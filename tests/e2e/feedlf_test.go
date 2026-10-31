package e2e

import (
	"strings"
	"testing"

	testkit "github.com/go-while/nntp-testkit"
)

// TestReaderFeedBareLFControlInBody: a feed peer sends an article with bare LF line ends
// whose body has a "Control:" line. The blank line ends the headers here too, so the body
// line is body text: the article is accepted as an ordinary article (235), lands in its
// group, and the feed connection stays up. Before the fix only a CRLF blank line ended the
// headers, the body line was taken for a Control: header, the overview code (which does stop
// at the bare LF) found no Control: header and dereferenced NULL: the feed process died and
// the peer got EOF.
func TestReaderFeedBareLFControlInBody(t *testing.T) {
	_, spool := spoolStore(t)
	s := NewReaderSite(t, ReaderOptions{Spool: spool})
	id := msgID(t)
	art := newArticle(id, "e2e.test", "bare LF", "Control: cancel <other@e2e.test>", "body")

	p := s.Peer(t)
	expect(t, p, 335, "IHAVE %s", id)
	send(t, p, strings.Join(art, "\n")+"\n.\n")
	status(t, p, 235, "IHAVE "+id+" (bare LF)")
	expect(t, p, 205, "QUIT")

	c := s.Reader(t)
	waitFor(t, testkit.Deadline(), "the article in GROUP e2e.test", func() bool {
		_, line := c.Cmd("GROUP e2e.test")
		f := strings.Fields(line)
		return len(f) > 1 && f[0] == "211" && f[1] != "0"
	})
	expect(t, c, 205, "QUIT")
}

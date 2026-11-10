package e2e

import (
	"bytes"
	"strings"
	"testing"

	"github.com/go-while/nntp-testkit/client"
)

// TestDeepRevWireFormatXref (L1-09): with "wireformat on" diablo stores an article as it
// came over the wire (CRLF line ends, dot-stuffed) and sends the stored bytes as they are.
//
// crlf: GenerateXRef() ended the Xref: line it adds with a bare LF also in this mode, so
// every stored article had one LF-only line among its CRLF headers and HEAD/ARTICLE sent it
// to the peer like that.
//
// barelf: LoadArticle() cut two bytes (CR LF) from the end of every header line it copies
// in this mode, also from a line the peer ended with a bare LF: "Newsgroups: e2e.test<LF>"
// became the group "e2e.tes".
func TestDeepRevWireFormatXref(t *testing.T) {
	conf := map[string]string{"wireformat": "on"}

	t.Run("crlf", func(t *testing.T) {
		s := deepRevXrefSite(t, conf)
		p := s.Peer(t)
		id := msgID(t)
		p.ExpectCmd(335, "IHAVE %s", id)
		p.MustSend(client.Stuff(newArticle(id, "e2e.test", "wire format", "body")))
		p.ExpectStatus(235, "IHAVE "+id)
		p.ExpectCmd(221, "HEAD %s", id)
		raw := p.RawUntilDot()
		p.ExpectCmd(205, "QUIT")
		if !bytes.Contains(raw, []byte("Xref: "+PathHost+" e2e.test:1\r\n")) {
			t.Errorf("no \"Xref: %s e2e.test:1<CR><LF>\" in the stored head: %q", PathHost, raw)
		}
		if n := bytes.Count(raw, []byte("\n")) - bytes.Count(raw, []byte("\r\n")); n != 0 {
			t.Errorf("%d line(s) of the stored head end with a bare LF: %q", n, raw)
		}
	})

	t.Run("barelf", func(t *testing.T) {
		s := deepRevXrefSite(t, conf)
		p := s.Peer(t)
		id := msgID(t)
		art := strings.Replace(client.Stuff(newArticle(id, "e2e.test", "wire format", "body")),
			"Newsgroups: e2e.test\r\n", "Newsgroups: e2e.test\n", 1)
		p.ExpectCmd(335, "IHAVE %s", id)
		p.MustSend(art)
		p.ExpectStatus(235, "IHAVE "+id)
		p.Strict = false // the stored head has the peer's own bare LF
		p.ExpectCmd(221, "HEAD %s", id)
		head := p.ReadDotLines()
		p.ExpectCmd(205, "QUIT")
		if x, want := headerValue(head, "Xref"), PathHost+" e2e.test:1"; x != want {
			t.Errorf("Xref of an article with \"Newsgroups: e2e.test<LF>\": %q, want %q", x, want)
		}
	})
}

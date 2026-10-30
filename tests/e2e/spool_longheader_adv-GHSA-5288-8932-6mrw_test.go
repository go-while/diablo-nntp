package e2e

import (
	"strings"
	"testing"

	"github.com/go-while/nntp-testkit/fakeserver"
)

// TestSpoolLongXrefPath (GHSA-5288-8932-6mrw): dreaderd rewrites the Xref: and Path: header
// of an article it fetches from the spool server into a buffer of 8192 bytes, and copied the
// header as long as the spool server sent it. A header over about 8 KB overflowed the stack of
// the reader process on HEAD or ARTICLE by message-id. The headers are cut to the buffer (as the cache path
// does), the client gets its answer and the reader keeps serving.
func TestSpoolLongXrefPath(t *testing.T) {
	id := msgID(t)
	headers := []string{
		"Path: " + strings.Repeat("p", 12000) + "!not-for-mail",
		"Xref: other.host e2e.test:1 " + strings.Repeat("a.b:1 ", 2500),
		"From: E2E Test <e2e@example.invalid>",
		"Newsgroups: e2e.test",
		"Subject: long headers",
		"Message-ID: " + id,
		"Date: Thu, 29 Oct 2026 12:00:00 +0000",
	}
	st := fakeserver.NewStore()
	st.AddGroup("e2e.test", "e2e test group")
	// The spool server answers HEAD and ARTICLE with the long headers. By message-id the reader
	// shows the headers the spool server sent (by article number it shows its overview), with
	// its own name put into the Path and the Xref, which is the code that has the buffer.
	article := append(append([]string{"220 1 " + id}, headers...), "", "body", ".")
	head := append(append([]string{"221 1 " + id}, headers...), ".")
	spool := fakeserver.New(t, fakeserver.Options{Store: st, Faults: fakeserver.Faults{
		Override: map[string][]string{"ARTICLE": article, "HEAD": head},
	}})
	s := NewReaderSite(t, ReaderOptions{Spool: spool})
	feedReader(t, s, st, "e2e.test", newArticle(id, "e2e.test", "long headers", "body"))

	c := s.Reader(t)
	defer expect(t, c, 205, "QUIT")
	expect(t, c, 211, "GROUP e2e.test")
	for _, cmd := range []string{"HEAD " + id, "ARTICLE " + id} {
		code, line := c.Cmd("%s", cmd)
		if code != 221 && code != 220 {
			t.Fatalf("%s: %d %q, want the article", cmd, code, line)
		}
		var path, xref string
		for _, l := range c.ReadDotLines() {
			switch {
			case strings.HasPrefix(l, "Path: "):
				path = l
			case strings.HasPrefix(l, "Xref: "):
				xref = l
			}
		}
		t.Logf("%s: Path %d bytes, Xref %d bytes", cmd, len(path), len(xref))
		if path == "" || len(path) > 8192 {
			t.Errorf("%s: Path header of %d bytes, want one of at most 8192", cmd, len(path))
		}
		if xref == "" || len(xref) > 8192 {
			t.Errorf("%s: Xref header of %d bytes, want one of at most 8192", cmd, len(xref))
		}
	}
	expect(t, c, 223, "STAT 1") // the reader is still there
}

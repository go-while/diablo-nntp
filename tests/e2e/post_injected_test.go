package e2e

import (
	"strings"
	"testing"

	"github.com/go-while/nntp-testkit/fakeserver"
)

// TestReaderPostRefusesInjected (RFC 5537 section 3.5 step 2, ISSUES.md F-07): an injecting
// agent must reject a proto-article that has an Injection-Info or an Xref header field, or a
// Path with the "POSTED" diag-keyword, the marks of an article that was already injected.
// Before, Xref was removed silently and the others were taken. A Path without POSTED, and a
// word that only contains POSTED, are still fine.
func TestReaderPostRefusesInjected(t *testing.T) {
	_, spool := spoolStore(t)
	post := fakeserver.New(t, fakeserver.Options{Handler: fakeserver.PeerHandler(&fakeserver.PeerOptions{})})
	s := NewReaderSite(t, ReaderOptions{Spool: spool, Post: post})

	postWith := func(extra string) (int, string) {
		t.Helper()
		c := s.Reader(t)
		defer c.Close()
		expect(t, c, 340, "POST")
		lines := []string{
			"From: E2E Poster <poster@example.invalid>",
			"Newsgroups: e2e.test",
			"Subject: injected?",
			"Message-ID: " + msgID(t),
		}
		if extra != "" {
			lines = append(lines, extra)
		}
		lines = append(lines, "", "body")
		send(t, c, strings.Join(lines, "\r\n")+"\r\n.\r\n")
		code, line, err := c.ReadStatus()
		if err != nil {
			t.Fatalf("POST with %q: %v", extra, err)
		}
		return code, line
	}

	for _, h := range []string{
		"Injection-Info: news.example.invalid; posting-host=\"client.example.invalid\"",
		"Xref: news.example.invalid e2e.test:1",
		"Path: news.example.invalid!.POSTED!not-for-mail",
		"Path: news.example.invalid!.POSTED.192.0.2.1!not-for-mail",
	} {
		if code, line := postWith(h); code != 441 {
			t.Errorf("POST with %q: %q, want 441", h, line)
		}
	}
	for _, h := range []string{
		"",
		"Path: news.example.invalid!not-for-mail",
		"Path: news.example.invalid!x.POSTEDNOT!not-for-mail",
	} {
		if code, line := postWith(h); code != 240 {
			t.Errorf("POST with %q: %q, want 240", h, line)
		}
	}
}

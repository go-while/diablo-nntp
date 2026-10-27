package e2e

// dreaderd: an article fed with LF-only line ends.

import (
	"testing"
)

// feedLFArticle IHAVEs an article whose lines end in LF alone, with body in place after the
// blank line, and returns the reply code.
func feedLFArticle(t *testing.T, s *Site, body string) int {
	t.Helper()
	p := s.dialFrom(t, PeerIP)
	defer p.Close()
	id := msgID(t)
	expect(t, p, 335, "IHAVE %s", id)
	send(t, p, "Path: x\nFrom: a@example.invalid\nNewsgroups: e2e.test\nSubject: s\nMessage-ID: "+id+
		"\nDate: Mon, 02 Jan 2006 15:04:05 -0700\n\n"+body+".\n")
	code, line, err := p.ReadStatus()
	if err != nil {
		t.Fatalf("reply to the article: %v", err)
	}
	t.Logf("reply %q", line)
	return code
}

// TestFeedLFOnlyBodyControl: a body line that looks like a Control header, in an article
// with LF-only line ends, is part of the body: the article is accepted and the feeder keeps
// serving.
func TestFeedLFOnlyBodyControl(t *testing.T) {
	_, spool := spoolStore(t)
	s := NewReaderSite(t, ReaderOptions{Spool: spool})
	if code := feedLFArticle(t, s, "body line\nControl: x\n"); code != 235 {
		t.Errorf("reply %d, want 235", code)
	}
	c := s.Reader(t)
	expect(t, c, 205, "QUIT")
	if code := feedLFArticle(t, s, "body line\n"); code != 235 {
		t.Errorf("article after it: reply %d, want 235", code)
	}
}

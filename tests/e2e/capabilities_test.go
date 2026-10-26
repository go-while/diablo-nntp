package e2e

import (
	"strings"
	"testing"

	testkit "github.com/go-while/nntp-testkit"
	"github.com/go-while/nntp-testkit/client"
	"github.com/go-while/nntp-testkit/conformance"
	"github.com/go-while/nntp-testkit/fakeserver"
)

// readCaps sends CAPABILITIES and returns the capability lines by keyword (upper case), with
// their arguments, and the lines in order.
func readCaps(tb testing.TB, c *client.Conn) (map[string][]string, []string) {
	tb.Helper()
	expect(tb, c, 101, "CAPABILITIES")
	lines := c.ReadDotLines()
	caps := map[string][]string{}
	for _, l := range lines {
		f := strings.Fields(l)
		if len(f) == 0 {
			tb.Fatalf("empty capability line in %q", lines)
		}
		caps[strings.ToUpper(f[0])] = f[1:]
	}
	return caps, lines
}

// TestReaderCapabilities: dreaderd answers CAPABILITIES (RFC 3977 §5.2) with what the
// connection can use, and everything it lists works. A reader connection (read and post):
// READER, OVER, LIST with ACTIVE ACTIVE.TIMES NEWSGROUPS OVERVIEW.FMT DISTRIB.PATS, POST; no
// MODE-READER, IHAVE, STREAMING or HDR (HDR has no message-id form yet). A feed-only connection (transit
// mode): IHAVE and STREAMING, no READER and no MODE-READER; MODE STREAM gets 203 there and
// 501 on the reader connection. Then nntp-testkit's conformance
// checks run against the reader, and none may fail (before the reply-code fixes for D-44 to
// D-47: listgroup, no-group, no-current-article and over-errors). Before, CAPABILITIES got
// "500 What?".
func TestReaderCapabilities(t *testing.T) {
	st, spool := spoolStore(t)
	post, _ := newPeer(t, &fakeserver.PeerOptions{})
	s := NewReaderSite(t, ReaderOptions{Spool: spool, Post: post})
	id := msgID(t)
	feedReader(t, s, st, "e2e.test", newArticle(id, "e2e.test", "capabilities", "body"))

	c := s.Reader(t)
	caps, lines := readCaps(t, c)
	if len(lines) == 0 || lines[0] != "VERSION 2" {
		t.Fatalf("first capability line %q, want \"VERSION 2\" (all: %q)", lines, lines)
	}
	for _, want := range []string{"READER", "OVER", "LIST", "POST", "IMPLEMENTATION"} {
		if _, ok := caps[want]; !ok {
			t.Errorf("reader connection: %s not advertised: %q", want, lines)
		}
	}
	for _, not := range []string{"MODE-READER", "IHAVE", "STREAMING", "HDR"} {
		if _, ok := caps[not]; ok {
			t.Errorf("reader connection: %s advertised: %q", not, lines)
		}
	}
	if impl := caps["IMPLEMENTATION"]; len(impl) == 0 || impl[0] != "Diablo" {
		t.Errorf("IMPLEMENTATION %q, want Diablo <version>", impl)
	}

	// Every advertised LIST keyword answers 215 with a list.
	for _, kw := range caps["LIST"] {
		expect(t, c, 215, "LIST %s", kw)
		c.ReadDotLines()
	}
	// OVER
	waitFor(t, testkit.Deadline(), "the article in GROUP e2e.test", func() bool {
		_, line := c.Cmd("GROUP e2e.test")
		f := strings.Fields(line)
		return len(f) > 1 && f[0] == "211" && f[1] != "0"
	})
	expect(t, c, 224, "OVER 1")
	if over := c.ReadDotLines(); len(over) != 1 || !strings.Contains(over[0], id) {
		t.Errorf("OVER 1: %q, want the article", over)
	}
	// POST
	expect(t, c, 340, "POST")
	send(t, c, wire([]string{
		"From: E2E Poster <poster@example.invalid>",
		"Newsgroups: e2e.test",
		"Subject: capabilities post",
		"Message-ID: " + msgID(t),
		"",
		"posted body",
	}))
	status(t, c, 240, "POST article")
	// MODE STREAM is refused in reader mode (no CHECK/TAKETHIS there, STREAMING not listed).
	expect(t, c, 501, "MODE STREAM")
	expect(t, c, 205, "QUIT")

	// A feed-only connection is in transit mode.
	p := s.dialFrom(t, PeerIP)
	fcaps, flines := readCaps(t, p)
	for _, want := range []string{"VERSION", "IHAVE", "STREAMING", "IMPLEMENTATION"} {
		if _, ok := fcaps[want]; !ok {
			t.Errorf("feed connection: %s not advertised: %q", want, flines)
		}
	}
	for _, not := range []string{"READER", "MODE-READER", "POST", "OVER"} {
		if _, ok := fcaps[not]; ok {
			t.Errorf("feed connection: %s advertised: %q", not, flines)
		}
	}
	expect(t, p, 203, "MODE STREAM")
	expect(t, p, 205, "QUIT")

	// nntp-testkit's RFC 3977 checks against the reader (no posting).
	rep := conformance.Run(t, conformance.Options{Addr: s.Addr, ReportOnly: true, Workers: 2})
	var failed []string
	for _, r := range rep.Results {
		if r.Status != conformance.Skip {
			t.Logf("conformance %-22s %s %s", r.Name, r.Status, r.Detail)
		}
		if r.Status == conformance.Fail {
			failed = append(failed, r.Name)
		}
	}
	if len(failed) > 0 {
		t.Errorf("conformance checks failed: %q", failed)
	}
}

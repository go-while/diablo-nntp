package e2e

// RFC 3977 / 4643 reply codes found by the conformance sweep (ISSUES D-44 to D-48).

import (
	"strings"
	"testing"
)

// TestReaderReplyCodes: dreaderd's reply codes for LISTGROUP (D-44), HDR and LIST HEADERS
// (D-45), and the error cases of NEXT/LAST, ARTICLE, OVER and LIST ACTIVE.TIMES (D-47).
// XHDR and XOVER keep their old replies.
func TestReaderReplyCodes(t *testing.T) {
	st, spool := spoolStore(t)
	s := NewReaderSite(t, ReaderOptions{Spool: spool})
	id := msgID(t)
	feedReader(t, s, st, "e2e.test", newArticle(id, "e2e.test", "reply codes", "body"))

	c := s.Reader(t)
	defer c.ExpectCmd(205, "QUIT")

	// No group selected: 412 (D-44, D-47).
	for _, cmd := range []string{"LISTGROUP", "NEXT", "LAST"} {
		c.ExpectCmd(412, "%s", cmd)
	}

	// An empty group: no current article, 420 (D-47).
	c.ExpectCmd(211, "GROUP e2e.other")
	for _, cmd := range []string{"ARTICLE", "HEAD", "BODY", "STAT", "NEXT", "LAST", "OVER", "HDR Subject"} {
		c.ExpectCmd(420, "%s", cmd)
	}

	// LISTGROUP: the GROUP line, then the numbers in the range (D-44).
	line := c.ExpectCmd(211, "LISTGROUP e2e.test")
	if f := strings.Fields(line); len(f) < 5 || f[1] != "1" || f[2] != "1" || f[3] != "1" || f[4] != "e2e.test" {
		t.Errorf("LISTGROUP e2e.test: %q, want \"211 1 1 1 e2e.test ...\"", line)
	}
	if got := c.ReadDotLines(); len(got) != 1 || got[0] != "1" {
		t.Errorf("LISTGROUP e2e.test list: %q, want [1]", got)
	}
	c.ExpectCmd(211, "LISTGROUP e2e.test 2-5")
	if got := c.ReadDotLines(); len(got) != 0 {
		t.Errorf("LISTGROUP e2e.test 2-5 list: %q, want none", got)
	}
	c.ExpectCmd(211, "LISTGROUP e2e.test 1-")
	if got := c.ReadDotLines(); len(got) != 1 || got[0] != "1" {
		t.Errorf("LISTGROUP e2e.test 1- list: %q, want [1]", got)
	}
	c.ExpectCmd(411, "LISTGROUP e2e.nonexistent")

	// ARTICLE with an argument that is neither a number nor a message-id: 501 (D-47).
	c.ExpectCmd(211, "GROUP e2e.test")
	for _, arg := range []string{"abc", "-1", "1x"} {
		c.ExpectCmd(501, "ARTICLE %s", arg)
	}

	// HDR: 225, an absent header is an empty value; XHDR unchanged (D-45).
	c.ExpectCmd(225, "HDR Subject 1")
	if got := c.ReadDotLines(); len(got) != 1 || got[0] != "1 reply codes" {
		t.Errorf("HDR Subject 1: %q, want [\"1 reply codes\"]", got)
	}
	c.ExpectCmd(225, "HDR X-Not-There 1")
	if got := c.ReadDotLines(); len(got) != 1 || strings.TrimSpace(got[0]) != "1" {
		t.Errorf("HDR X-Not-There 1: %q, want [\"1 \"] (empty value)", got)
	}
	c.ExpectCmd(221, "XHDR Subject 1")
	c.ReadDotLines()
	// HDR by message-id works (D-193): "<msgid> <value>".
	c.ExpectCmd(225, "HDR Subject %s", id)
	if got := c.ReadDotLines(); len(got) != 1 || got[0] != id+" reply codes" {
		t.Errorf("HDR Subject %s: %q, want [%q]", id, got, id+" reply codes")
	}

	// LIST HEADERS (D-45) and LIST ACTIVE.TIMES (D-47).
	c.ExpectCmd(215, "LIST HEADERS")
	if got := c.ReadDotLines(); len(got) != 1 || got[0] != ":" {
		t.Errorf("LIST HEADERS: %q, want [\":\"]", got)
	}
	c.ExpectCmd(215, "LIST HEADERS MSGID")
	c.ReadDotLines()
	c.ExpectCmd(215, "LIST ACTIVE.TIMES")
	for _, l := range c.ReadDotLines() {
		if f := strings.Fields(l); len(f) != 3 || !IsDigits(f[1]) {
			t.Errorf("LIST ACTIVE.TIMES line %q, want \"group time creator\"", l)
		}
	}

	// OVER: 423 for a range without articles; XOVER keeps the empty 224 list (D-47).
	for _, r := range []string{"1000-1010", "3-1", "2-"} {
		c.ExpectCmd(423, "OVER %s", r)
	}
	c.ExpectCmd(224, "OVER 1")
	if got := c.ReadDotLines(); len(got) != 1 {
		t.Errorf("OVER 1: %d lines, want 1", len(got))
	}
	c.ExpectCmd(224, "XOVER 1000-1010")
	if got := c.ReadDotLines(); len(got) != 0 {
		t.Errorf("XOVER 1000-1010: %q, want an empty list", got)
	}
}

// IsDigits reports whether s is a non-empty string of digits.
func IsDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// AuthReaderAccess requires AUTHINFO (user fred, password fredspass) from 127.0.0.1.
const AuthReaderAccess = `vserverdef DEFAULT
    hostname ` + PathHost + `
end
authdef fred
    user fred
    pass fredspass
end
readerdef userfred
    read yes
    post yes
    auth fred
end
readerdef feed
    feed yes
end
access 127.0.0.1 userfred
access ` + PeerIP + ` feed
`

// TestReaderAuthinfoCodes: AUTHINFO reply codes (RFC 4643, D-46): PASS before USER is 482,
// a wrong password 481 (not "502 Access denied"), and AUTHINFO after a successful login 502.
func TestReaderAuthinfoCodes(t *testing.T) {
	_, spool := spoolStore(t)
	s := NewReaderSite(t, ReaderOptions{Spool: spool, Access: AuthReaderAccess})

	c := s.Reader(t)
	c.ExpectCmd(482, "AUTHINFO PASS fredspass")
	c.ExpectCmd(381, "AUTHINFO USER fred")
	c.ExpectCmd(481, "AUTHINFO PASS wrong")

	c2 := s.Reader(t)
	c2.ExpectCmd(381, "AUTHINFO USER fred")
	c2.ExpectCmd(281, "AUTHINFO PASS fredspass")
	c2.ExpectCmd(502, "AUTHINFO USER fred")
	c2.ExpectCmd(205, "QUIT")
}

// TestFeederReplyCodes: diablo is a transit-only server: MODE READER is 502, and IHAVE or
// CHECK without a message-id is 501 (D-48).
func TestFeederReplyCodes(t *testing.T) {
	s := NewSite(t, SiteOptions{})
	p := s.Peer(t)
	p.ExpectCmd(502, "MODE READER")
	p.ExpectCmd(501, "IHAVE")
	p.ExpectCmd(203, "MODE STREAM")
	p.ExpectCmd(501, "CHECK")
	p.ExpectCmd(205, "QUIT")
}

package e2e

// XHDR/XOVER/HDR/OVER by message-id (D-193).

import (
	"strings"
	"testing"

	testkit "github.com/go-while/nntp-testkit"
)

// TestXhdrXoverByMessageId: XHDR <hdr> <msgid> returns that article's header
// value with the message-id as the first field (RFC 2980 2.6), XOVER <msgid>
// returns its overview line, and the RFC 3977 HDR/OVER forms work the same way.
// An unknown message-id is honestly 430, not an empty 221/224 list.
func TestXhdrXoverByMessageId(t *testing.T) {
	st, spool := spoolStore(t)
	s := NewReaderSite(t, ReaderOptions{Spool: spool})
	id := msgID(t)
	feedReader(t, s, st, "e2e.test", newArticle(id, "e2e.test", "msgid subject", "body"))

	c := s.Reader(t)
	defer c.ExpectCmd(205, "QUIT")
	testkit.WaitFor(t, testkit.Deadline(), "the article in GROUP e2e.test", func() bool {
		_, line := c.Cmd("GROUP e2e.test")
		f := strings.Fields(line)
		return len(f) > 1 && f[0] == "211" && f[1] != "0"
	})

	// XHDR by message-id: "<msgid> <value>" (RFC 2980 2.6).
	c.ExpectCmd(221, "XHDR Subject %s", id)
	if got := c.ReadDotLines(); len(got) != 1 || got[0] != id+" msgid subject" {
		t.Errorf("XHDR Subject %s: %q, want [%q]", id, got, id+" msgid subject")
	}

	// XOVER by message-id: the message-id is the first field.
	c.ExpectCmd(224, "XOVER %s", id)
	if got := c.ReadDotLines(); len(got) != 1 || !strings.HasPrefix(got[0], "0\t") {
		t.Errorf("XOVER %s: %q, want one line starting with %q", id, got, "0\t")
	}

	// HDR and OVER by message-id (RFC 3977 8.3, 8.5) do the same.
	c.ExpectCmd(225, "HDR Subject %s", id)
	if got := c.ReadDotLines(); len(got) != 1 || got[0] != "0 msgid subject" {
		t.Errorf("HDR Subject %s: %q, want [%q]", id, got, "0 msgid subject")
	}
	c.ExpectCmd(224, "OVER %s", id)
	if got := c.ReadDotLines(); len(got) != 1 || !strings.HasPrefix(got[0], "0\t") {
		t.Errorf("OVER %s: %q, want one line starting with %q", id, got, "0\t")
	}

	// After a message-id list, a range list shows article numbers again: the
	// message-id must not stay behind as the first field.
	c.ExpectCmd(221, "XHDR Subject %s", id)
	c.ReadDotLines()
	c.ExpectCmd(221, "XHDR Subject 1-")
	for _, l := range c.ReadDotLines() {
		if strings.HasPrefix(l, "<") {
			t.Errorf("XHDR Subject 1- after a message-id list: %q starts with a message-id", l)
		}
	}

	// An unknown message-id is "not here", not an empty list.
	for _, cmd := range []string{"XHDR Subject", "XOVER", "HDR Subject", "OVER"} {
		c.ExpectCmd(430, "%s %s", cmd, "<nonexistent@e2e.test>")
	}
}

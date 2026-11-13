package e2e

import (
	"strings"
	"testing"
	"time"

	testkit "github.com/go-while/nntp-testkit"
	"github.com/go-while/nntp-testkit/fakeserver"
)

// deepRevBackendGot400 returns the first line the reader sent to its spool server that is a
// "400 ..." status (a server-to-client line that has no business on that connection), or "".
func deepRevBackendGot400(spool *fakeserver.Server) string {
	for _, l := range spool.Lines() {
		if strings.HasPrefix(l, "400 ") {
			return l
		}
	}
	return ""
}

// TestDeepRevSpoolLinkClientTimeout (L4-01): when the access cache changes, a reader process
// re-applies the access definitions to all its connections, and it did that to its links to
// the spool servers too. A link has no reader name, so it got the DEFAULT readerdef. The
// built-in DEFAULT sets nothing, but a "readerdef DEFAULT" in dreader.access (the documented
// way to change the defaults) with idletimeout or sessiontimeout then applied to the link:
// the reader wrote "400 <host>: Idle timeout." to the spool server and dropped the link
// without the bookkeeping of a server that went away, so the link was not set up again, and
// article requests waited for a spool that never came.
//
// A site with "readerdef DEFAULT / idletimeout 5": after an access reload and 75 idle
// seconds on the link (the reader checks all connections every 30 s), the spool server must
// not have been sent a "400" line and a BODY must still be served.
//
// Takes about 80 s.
func TestDeepRevSpoolLinkClientTimeout(t *testing.T) {
	access := "readerdef DEFAULT\n    idletimeout 5\nend\n" + DefaultReaderAccess
	st, spool := spoolStore(t)
	s := NewReaderSite(t, ReaderOptions{Spool: spool, Access: access})
	id := msgID(t)
	feedReader(t, s, st, "e2e.test", newArticle(id, "e2e.test", "spool link", "the body"))

	c := s.Reader(t)
	testkit.WaitFor(t, testkit.Deadline(), "the article in GROUP e2e.test", func() bool {
		_, line := c.Cmd("GROUP e2e.test")
		f := strings.Fields(line)
		return len(f) > 1 && f[0] == "211" && f[1] != "0"
	})
	// The link works (and exists) before the reload.
	c.ExpectCmd(222, "BODY %s", id)
	if body := c.ReadDotLines(); len(body) != 1 || body[0] != "the body" {
		t.Fatalf("BODY before the reload: %q", body)
	}

	// A new access cache: the file's time must be in a later second than the first load.
	time.Sleep(1500 * time.Millisecond)
	s.WriteFile(t, "dreader.access", access+"# reloaded by the test\n")

	// Commands keep the reader's loop turning (it looks at the access cache before each
	// command and counts its timers every tenth pass). The link itself stays idle.
	limit := time.Now().Add(75 * time.Second)
	for time.Now().Before(limit) {
		c.ExpectCmd(111, "DATE")
		if l := deepRevBackendGot400(spool); l != "" {
			t.Fatalf("the reader applied the client idle timeout of readerdef DEFAULT to its spool server link: it sent %q to the spool server", l)
		}
		time.Sleep(500 * time.Millisecond)
	}

	c.Deadline = 20 * time.Second
	code, line, err := c.TryCmd("BODY %s", id)
	if err != nil || code != 222 {
		t.Fatalf("BODY after 75 s: %q (%v), want 222: the reader has lost its spool server link", line, err)
	}
	if body := c.ReadDotLines(); len(body) != 1 || body[0] != "the body" {
		t.Errorf("BODY after 75 s: %q", body)
	}
	c.ExpectCmd(205, "QUIT")
}

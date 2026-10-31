package e2e

import (
	"net"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-while/nntp-testkit/client"
	"github.com/go-while/nntp-testkit/proc"
)

// TestFeedNoXrefLongXRefHost: WriteOverview() sized its synthetic Xref line
// from the vserver's cluster name (<= 63 bytes) but wrote the configured
// readerxrefhost, an unbounded strdup.  A feed article without an Xref: made
// the sprintf overflow the zalloc pool.  The fix sizes the buffer from the
// string actually written.
//
// The positive control uses the ordinary short host.  The attack site starts
// dreaderd with a 120 KiB -X value: the old sprintf writes far past the small
// pool chunk (the feed child dies / the IHAVE gets EOF), the fixed build
// accepts the article and the overview carries the long Xref.
func TestFeedNoXrefLongXRefHost(t *testing.T) {
	// Positive control: a normal short Xref host.
	st, spool := spoolStore(t)
	s := NewReaderSite(t, ReaderOptions{Spool: spool})
	id := msgID(t)
	feedReader(t, s, st, "e2e.test", newArticle(id, "e2e.test", "short xref host", "body"))

	r := s.Reader(t)
	expect(t, r, 211, "GROUP e2e.test")
	if over := overOne(t, r); !strings.Contains(over, "Xref: "+PathHost+" e2e.test:1") {
		t.Errorf("OVER 1 does not carry the generated Xref: %q", over)
	}
	expect(t, r, 205, "QUIT")

	// The attack: a 120 KiB configured Xref host, article without Xref:.
	const hostLen = 120 * 1024
	longHost := strings.Repeat("X", hostLen)
	st2, spool2 := spoolStore(t)
	s2 := NewReaderSite(t, ReaderOptions{Spool: spool2, NoStart: true})
	_, port, err := net.SplitHostPort(s2.Addr)
	if err != nil {
		t.Fatal(err)
	}
	s2.Proc = proc.StartTCP(t, proc.Options{
		Path: filepath.Join(s2.Bin, "dreaderd"),
		Args: []string{"-d", "-p", PathHost, "-h", PathHost,
			"-P", "[127.0.0.1]:" + port, "-X", longHost},
		Env: s2.Env,
		Dir: s2.Dir,
	}, s2.Addr)

	id2 := msgID(t)
	feedReader(t, s2, st2, "e2e.test", newArticle(id2, "e2e.test", "long xref host", "body"))

	r2 := s2.Reader(t)
	expect(t, r2, 211, "GROUP e2e.test")
	if over := overOne(t, r2); !strings.Contains(over, "Xref: "+longHost) {
		t.Errorf("OVER 1 does not carry the %d byte Xref host", hostLen)
	}
	expect(t, r2, 205, "QUIT")
}

// overOne asks for article 1's overview and returns the single line.
func overOne(t *testing.T, c *client.Conn) string {
	t.Helper()
	expect(t, c, 224, "OVER 1")
	lines := c.ReadDotLines()
	if len(lines) != 1 {
		t.Fatalf("OVER 1 returned %d lines, want 1", len(lines))
	}
	return lines[0]
}

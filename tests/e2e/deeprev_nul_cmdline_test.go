package e2e

import (
	"bufio"
	"net"
	"strings"
	"testing"
	"time"
)

// nulLineConn is a raw connection for command lines with a NUL byte: the testkit client
// writes strings too, but this one reads with a short deadline, so a line that gets no
// answer fails in seconds.
type nulLineConn struct {
	c net.Conn
	r *bufio.Reader
}

func dialNulLine(t *testing.T, addr, from string) *nulLineConn {
	t.Helper()
	d := net.Dialer{LocalAddr: &net.TCPAddr{IP: net.ParseIP(from)}, Timeout: 10 * time.Second}
	c, err := d.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("dial %s from %s: %v", addr, from, err)
	}
	t.Cleanup(func() { c.Close() })
	n := &nulLineConn{c: c, r: bufio.NewReader(c)}
	if g, err := n.line(); err != nil || !strings.HasPrefix(g, "20") {
		t.Fatalf("greeting from %s: %q %v", from, g, err)
	}
	return n
}

func (n *nulLineConn) line() (string, error) {
	n.c.SetReadDeadline(time.Now().Add(5 * time.Second))
	l, err := n.r.ReadString('\n')
	return strings.TrimRight(l, "\r\n"), err
}

// send writes raw and returns the first line of the answer.
func (n *nulLineConn) send(raw string) (string, error) {
	n.c.SetWriteDeadline(time.Now().Add(5 * time.Second))
	if _, err := n.c.Write([]byte(raw)); err != nil {
		return "", err
	}
	return n.line()
}

// nulCommandLines sends each of the bad lines on a connection of its own (from "from"),
// expects 501, then sends good and expects an answer that starts with want.
func nulCommandLines(t *testing.T, addr, from, id, good, want string) {
	t.Helper()
	for _, bad := range []string{
		"DATE\x00x\r\n",
		"\x00ARTICLE " + id + "\r\n",
		"ARTICLE " + id + "\x00junk\r\n",
	} {
		n := dialNulLine(t, addr, from)
		l, err := n.send(bad)
		if err != nil || !strings.HasPrefix(l, "501 ") {
			t.Errorf("from %s: %q: %q %v, want 501 (no NUL in a command line)", from, bad, l, err)
		} else if l, err := n.send(good); err != nil || !strings.HasPrefix(l, want) {
			t.Errorf("from %s: %q after %q: %q %v, want %s (the connection stays usable)", from, good, bad, l, err, want)
		}
		// ended before the next one: dreaderd serves one connection of a feed at a time
		n.send("QUIT\r\n")
		n.c.Close()
	}
}

// TestReaderNulCommandLine: a command line cannot hold a NUL byte (RFC 3977 9.2, 9.8) and must
// not be cut and then interpreted (3.2.1).
// dreaderd used the line as a C string, so it ended at the NUL: "ARTICLE <id>" NUL junk
// was served and "DATE" NUL x answered, and a line that starts with NUL got no answer at
// all. Now each answers 501 and the connection takes the next command, on a reader's
// connection and on a feed's (PeerIP).
func TestReaderNulCommandLine(t *testing.T) {
	st, spool := spoolStore(t)
	s := NewReaderSite(t, ReaderOptions{Spool: spool})
	id := msgID(t)
	feedReader(t, s, st, "e2e.test", newArticle(id, "e2e.test", "nul in a command", "body"))

	nulCommandLines(t, s.Addr, "127.0.0.1", id, "DATE\r\n", "111 ")
	nulCommandLines(t, s.Addr, PeerIP, id, "DATE\r\n", "111 ")
}

// TestFeederNulCommandLine: the same for a feed session of diablo, which also cut the line
// at the NUL and did not answer a line that starts with NUL.
func TestFeederNulCommandLine(t *testing.T) {
	s := NewSite(t, SiteOptions{})
	nulCommandLines(t, s.Addr, PeerIP, msgID(t), "MODE STREAM\r\n", "203 ")
}

// TestReaderNulCommandLineAtEOF: the last line of a client that closes its side may have no
// newline. MBReadLine() then returns all of it and puts its NUL behind it; the check took the
// last byte for the NUL of the newline and did not look at it, so "ARTICLE <id>" NUL at the
// end was served. Each such line must get 501; the same lines without the NUL are answered.
func TestReaderNulCommandLineAtEOF(t *testing.T) {
	st, spool := spoolStore(t)
	s := NewReaderSite(t, ReaderOptions{Spool: spool})
	id := msgID(t)
	feedReader(t, s, st, "e2e.test", newArticle(id, "e2e.test", "nul at eof", "body"))

	last := func(raw string) string {
		t.Helper()
		n := dialNulLine(t, s.Addr, "127.0.0.1")
		n.c.SetWriteDeadline(time.Now().Add(5 * time.Second))
		if _, err := n.c.Write([]byte(raw)); err != nil {
			t.Fatal(err)
		}
		n.c.(*net.TCPConn).CloseWrite()
		l, _ := n.line()
		return l
	}
	for _, tc := range []struct{ raw, want string }{
		{"DATE\x00", "501 "},
		{"ARTICLE " + id + "\x00", "501 "},
		{"DATE", "111 "},
		{"ARTICLE " + id, "220 "},
	} {
		if l := last(tc.raw); !strings.HasPrefix(l, tc.want) {
			t.Errorf("%q as the last line, without a newline: %q, want %s", tc.raw, l, tc.want)
		}
	}
}

// TestReaderNulProxyLine: the first line of a client through a proxy (haproxy: a PROXY v1
// header; useproxied: "proxied addr:port") is handled before the NUL check of the commands,
// and was used as a C string too: a header with a NUL and anything behind it was taken.
// It is refused like any other bad header now (400 and close).
func TestReaderNulProxyLine(t *testing.T) {
	const haproxy, proxied = "127.0.0.3", "127.0.0.4"
	_, spool := spoolStore(t)
	access := DefaultReaderAccess +
		"readerdef hp\n    read yes\n    haproxy yes\nend\naccess " + haproxy + " hp\n" +
		"readerdef up\n    read yes\n    useproxied yes\n    proxyfrom " + proxied + "\nend\naccess " + proxied + " up\n"
	s := NewReaderSite(t, ReaderOptions{Spool: spool, Access: access})

	const hdr = "PROXY TCP4 127.0.0.1 127.0.0.1 40000 119"
	const badHeader = "400 " + PathHost + ": failed to read haproxy PROXY v1 header."
	if got := rawLineFrom(t, haproxy, s.Addr, hdr+"\x00junk\r\n"); got != badHeader {
		t.Errorf("a PROXY header with a NUL: got %q, want %q", got, badHeader)
	}
	if got := rawLineFrom(t, haproxy, s.Addr, hdr+"\r\n"); !strings.HasPrefix(got, "200 ") {
		t.Errorf("the PROXY header without the NUL: got %q, want the 200 greeting", got)
	}

	proxiedLine := func(line string) string {
		t.Helper()
		n := dialNulLine(t, s.Addr, proxied)
		l, _ := n.send(line)
		return l
	}
	const badProxied = "400 " + PathHost + ": Proxy authentication failure."
	if got := proxiedLine("proxied 127.0.0.1:1234\x00junk\r\n"); got != badProxied {
		t.Errorf("a proxied line with a NUL: got %q, want %q", got, badProxied)
	}
	if got := proxiedLine("proxied 127.0.0.1:1234\r\n"); strings.HasPrefix(got, "400 ") {
		t.Errorf("the proxied line without the NUL was refused: %q", got)
	}
}

package e2e

import (
	"bufio"
	"net"
	"strings"
	"testing"
	"time"
)

// rawLineFrom connects to addr from the local address from, sends send (if not empty) and
// returns the first line dreaderd answers, "" on EOF or timeout.
func rawLineFrom(t *testing.T, from, addr, send string) string {
	t.Helper()
	d := net.Dialer{LocalAddr: &net.TCPAddr{IP: net.ParseIP(from)}, Timeout: 2 * time.Second}
	conn, err := d.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("connect %s from %s: %v", addr, from, err)
	}
	defer conn.Close()
	if send != "" {
		if _, err := conn.Write([]byte(send)); err != nil {
			t.Fatalf("write: %v", err)
		}
	}
	conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	line, _ := bufio.NewReader(conn).ReadString('\n')
	return strings.TrimRight(line, "\r\n")
}

// TestReaderHAProxyEmptyLine: with haproxy on, the first line a client sends must be a PROXY
// v1 header. An empty first line is refused with 400 like any other bad header, and the
// reader process keeps serving. Fails before: the empty line gave no word to compare, the
// reader process died and the client got no answer. The haproxy readerdef is for 127.0.0.3
// only, so the start-up check from 127.0.0.1 still gets a greeting.
func TestReaderHAProxyEmptyLine(t *testing.T) {
	const proxy = "127.0.0.3"
	_, spool := spoolStore(t)
	access := DefaultReaderAccess + "readerdef hp\n    read yes\n    haproxy yes\nend\naccess " + proxy + " hp\n"
	s := NewReaderSite(t, ReaderOptions{Spool: spool, Access: access})

	const want = "400 " + PathHost + ": failed to read haproxy PROXY v1 header."
	for i := 0; i < 2; i++ {
		if got := rawLineFrom(t, proxy, s.Addr, "\r\n"); got != want {
			t.Fatalf("empty first line, connection %d: got %q, want %q", i+1, got, want)
		}
	}
	if got := rawLineFrom(t, proxy, s.Addr, "PROXY TCP4 127.0.0.1 127.0.0.1 40000 119\r\n"); !strings.HasPrefix(got, "200 ") {
		t.Errorf("a PROXY header after the empty lines: got %q, want the 200 greeting", got)
	}
}

// TestReaderWelcomeLong: a welcome text whose expansion is longer than the greeting buffer is
// cut at its end, and a welcome ending in '%' ends there. Before, the length snprintf()
// wanted was added unchecked, so the closing '\0' was written past the buffer, and a '%' at
// the end stepped over the end of the text.
func TestReaderWelcomeLong(t *testing.T) {
	org := strings.Repeat("o", 127)
	for _, tc := range []struct {
		name, welcome, want string
	}{
		{"cut in an expansion", strings.Repeat("%o", 4) + "%h", "200 " + strings.Repeat("o", 508) + "e2e (posting ok)"},
		{"cut in the text", strings.Repeat("%o", 4) + "abcd", "200 " + strings.Repeat("o", 508) + "abc (posting ok)"},
		{"far past the end", strings.Repeat("%o", 6), "200 " + strings.Repeat("o", 511) + " (posting ok)"},
		{"percent at the end", "hello %h%", "200 hello " + PathHost + " (posting ok)"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, spool := spoolStore(t)
			access := strings.Replace(DefaultReaderAccess, "    hostname "+PathHost+"\n",
				"    hostname "+PathHost+"\n    organization "+org+"\n    welcome "+tc.welcome+"\n", 1)
			s := NewReaderSite(t, ReaderOptions{Spool: spool, Access: access})
			for i := 0; i < 2; i++ {
				if got := strings.TrimRight(readerGreeting(s.Addr), "\r\n"); got != tc.want {
					t.Fatalf("greeting %d: got %q (%d bytes), want %q (%d bytes)",
						i+1, got, len(got), tc.want, len(tc.want))
				}
			}
		})
	}
}

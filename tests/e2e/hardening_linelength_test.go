package e2e

// dreaderd: the length of a line received from a client is bounded.

import (
	"bytes"
	"io"
	"net"
	"strings"
	"testing"
	"time"
)

// clientMaxLine is the longest line dreaderd takes from a client, newline included
// (CLIENT_MAXLINE in dreaderd/defs.h).
const clientMaxLine = 1024 * 1024

// TestReaderLongLineClosed sends data without a line end: dreaderd must close the
// connection once the line passes the limit, not buffer it.
func TestReaderLongLineClosed(t *testing.T) {
	_, spool := spoolStore(t)
	s := NewReaderSite(t, ReaderOptions{Spool: spool})
	c, err := net.Dial("tcp", s.Addr)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	chunk := bytes.Repeat([]byte("A"), 64*1024)
	// 4 limits' worth: a server that keeps buffering takes all of it
	for i := 0; i < 4*clientMaxLine/len(chunk); i++ {
		c.SetWriteDeadline(time.Now().Add(20 * time.Second))
		if _, err := c.Write(chunk); err != nil {
			break // already closed by the server
		}
	}
	// the greeting, then EOF or a reset
	c.SetReadDeadline(time.Now().Add(10 * time.Second))
	_, err = io.Copy(io.Discard, c)
	if ne, ok := err.(net.Error); ok && ne.Timeout() {
		t.Fatalf("connection still open after %d bytes without a line end", 4*clientMaxLine)
	}
	// the reader still serves others
	c2 := s.Reader(t)
	expect(t, c2, 205, "QUIT")
}

// TestReaderLongLineAtLimit checks the edge: a command line of exactly the limit
// (newline included) is answered and the connection stays usable; a line one byte
// longer closes the connection.
func TestReaderLongLineAtLimit(t *testing.T) {
	_, spool := spoolStore(t)
	s := NewReaderSite(t, ReaderOptions{Spool: spool})

	c := s.Reader(t)
	send(t, c, "X"+strings.Repeat("a", clientMaxLine-3)+"\r\n")
	code, line, err := c.ReadStatus()
	if err != nil || code/100 != 5 {
		t.Fatalf("line of exactly %d bytes: %d %q err=%v, want a 5xx reply", clientMaxLine, code, line, err)
	}
	expect(t, c, 111, "DATE")
	expect(t, c, 205, "QUIT")

	c = s.Reader(t)
	send(t, c, "X"+strings.Repeat("a", clientMaxLine-2)+"\r\n")
	if code, line, err := c.ReadStatus(); err == nil {
		t.Fatalf("line of %d bytes answered: %d %q, want the connection closed", clientMaxLine+1, code, line)
	} else if ne, ok := err.(net.Error); ok && ne.Timeout() {
		t.Fatalf("line of %d bytes: connection still open", clientMaxLine+1)
	}
}

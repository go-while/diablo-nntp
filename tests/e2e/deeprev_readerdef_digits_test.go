package e2e

import (
	"net"
	"strings"
	"testing"
	"time"

	"github.com/go-while/nntp-testkit/client"
)

// TestDeepRevReaderdefNameWithDigits (L6-14): an authentication backend can refuse a client
// with a line "110 NNN text"; the worker passes "NNN text" to the main process in the field
// that otherwise holds the name of the client's readerdef, and the main process took any
// value that starts with three digits for such a refusal. A readerdef called "100mbit" (or
// "404") was therefore sent to every client of its access line as the greeting, "100mbit",
// and the connection was closed.
//
// The readerdef is used by PeerIP only, so the harness (127.0.0.1) can start the site. A
// client from PeerIP must get a normal greeting and an answer to a command.
func TestDeepRevReaderdefNameWithDigits(t *testing.T) {
	access := `vserverdef DEFAULT
    hostname ` + PathHost + `
end
readerdef rp
    read yes
    post yes
end
readerdef 100mbit
    read yes
end
access 127.0.0.1 rp
access ` + PeerIP + ` 100mbit
`
	_, spool := spoolStore(t)
	s := NewReaderSite(t, ReaderOptions{Spool: spool, Access: access})

	d := net.Dialer{LocalAddr: &net.TCPAddr{IP: net.ParseIP(PeerIP)}, Timeout: 10 * time.Second}
	nc, err := d.Dial("tcp", s.Addr)
	if err != nil {
		t.Fatalf("dial %s from %s: %v", s.Addr, PeerIP, err)
	}
	c := client.New(t, nc)
	c.Deadline = 15 * time.Second
	greeting, err := c.ReadGreeting()
	if err != nil {
		t.Fatalf("a client of readerdef 100mbit got no greeting: %v (%q)", err, greeting)
	}
	if !strings.HasPrefix(greeting, "200 ") && !strings.HasPrefix(greeting, "201 ") {
		t.Fatalf("a client of readerdef 100mbit got the greeting %q, want 200 or 201: the name of the readerdef was taken for a refusal line of an authentication backend",
			greeting)
	}
	c.ExpectCmd(111, "DATE")
	c.ExpectCmd(205, "QUIT")
}

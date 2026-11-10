package e2e

import (
	"net"
	"strings"
	"testing"
	"time"

	"github.com/go-while/nntp-testkit/client"
)

// TestDeepRevRatelimitRangeEqualBounds (L4-03): "ratelimitrange low-high" in a readerdef
// varies the rate limit of a session between low and high percent. dreader.access accepted
// low == high ("80-80"), and the reader then computed "random % (high - low)" for every write
// to such a client (dreaderd/mbuf.c, MBFlush()): a division by zero, SIGFPE on x86, which
// ends the reader process and with it every client it serves; the next client of that
// readerdef ends the next reader.
//
// The readerdef is used by PeerIP only, so the harness (127.0.0.1) can start the site. A
// client from PeerIP must get its greeting and an answer to a command.
func TestDeepRevRatelimitRangeEqualBounds(t *testing.T) {
	access := `vserverdef DEFAULT
    hostname ` + PathHost + `
end
readerdef rp
    read yes
    post yes
end
readerdef limited
    read yes
    ratelimit 100000
    ratelimitrange 80-80
end
access 127.0.0.1 rp
access ` + PeerIP + ` limited
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
		t.Fatalf("a client of the readerdef with ratelimitrange 80-80 got no greeting: %v (%q)\n"+
			"the reader process died on its first write to the client (division by zero in MBFlush):\n%s",
			err, greeting, s.logOutput())
	}
	if !strings.HasPrefix(greeting, "200") && !strings.HasPrefix(greeting, "201") {
		t.Fatalf("greeting %q, want 200 or 201", greeting)
	}
	c.ExpectCmd(111, "DATE")
	c.ExpectCmd(205, "QUIT")

	// and a client of another readerdef is served as before
	r := s.Reader(t)
	r.ExpectCmd(111, "DATE")
	r.ExpectCmd(205, "QUIT")
}

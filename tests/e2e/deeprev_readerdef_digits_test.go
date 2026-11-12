package e2e

import (
	"net"
	"path/filepath"
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

// TestDeepRevReaderdefDigitsBareCode: the main process takes a value that starts with three
// digits for a refusal only when a space follows ("502 Go away"). A file backend entry whose
// third field is a bare code ("502") or a code and a TAB is then a readerdef name, one that
// does not exist. Such a login must still be refused, not let in with another readerdef.
func TestDeepRevReaderdefDigitsBareCode(t *testing.T) {
	_, spool := spoolStore(t)
	s := NewReaderSite(t, ReaderOptions{Spool: spool, NoStart: true,
		Files: map[string]string{"userpass": "bare:pw:502\ntab:pw:502\tGo away\nsp:pw:502 Go away\n"}})
	access := `vserverdef DEFAULT
    hostname ` + PathHost + `
end
authdef byfile
    file ` + filepath.Join(s.Dir, "userpass") + `
end
readerdef full
    read yes
    post yes
end
readerdef userfile
    read yes
    auth byfile
end
access 127.0.0.1 userfile
`
	s.WriteFile(t, "dreader.access", access)
	startReader(t, s, "[127.0.0.1]:"+s.Addr[strings.LastIndex(s.Addr, ":")+1:])
	for _, u := range []string{"sp", "bare", "tab"} {
		c := s.Reader(t)
		c.ExpectCmd(381, "AUTHINFO USER %s", u)
		code, line := c.Cmd("AUTHINFO PASS pw")
		t.Logf("user %s: AUTHINFO PASS -> %d %q", u, code, line)
		if code == 281 {
			t.Errorf("user %s, whose file entry is a refusal (\"502...\"), was let in: %d %q", u, code, line)
		}
		c.Close()
	}
}

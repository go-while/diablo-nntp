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
// value that starts with three digits for such a refusal. A readerdef called "100mbit" was
// therefore sent to every client of its access line as the greeting, "100mbit", and the
// connection was closed. (A name of just three digits, "404", is a refusal and stays one.)
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

// refusalSite starts a reader whose clients from 127.0.0.1 log in through a file authdef
// with the entries in userpass, all with the password "pw". The admin defines readerdef
// DEFAULT with read access: SetAuthDetails() falls back to that first record for a name no
// readerdef has, so a refusal that is taken for a readerdef name lets the user in. Readerdef
// 100mbit allows posting and DEFAULT does not, so MODE READER (200 or 201) tells which of
// the two a login got.
func refusalSite(t *testing.T, userpass string) *Site {
	t.Helper()
	_, spool := spoolStore(t)
	s := NewReaderSite(t, ReaderOptions{Spool: spool, NoStart: true,
		Files: map[string]string{"userpass": userpass}})
	access := `vserverdef DEFAULT
    hostname ` + PathHost + `
end
authdef byfile
    file ` + filepath.Join(s.Dir, "userpass") + `
end
readerdef DEFAULT
    read yes
    post no
end
readerdef 100mbit
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
	return s
}

// refusalLogin logs in as user with the password "pw" and returns the raw reply line to
// AUTHINFO PASS, unparsed (a reply such as "502", TAB, "Go away" is not a status line the
// client would parse, but it is still a refusal). When the reply is 281 it also returns the
// reply to GROUP e2e.test.
func refusalLogin(t *testing.T, s *Site, user string) (line, group string) {
	t.Helper()
	c := s.Reader(t)
	defer c.Close()
	c.ExpectCmd(381, "AUTHINFO USER %s", user)
	if err := c.Send("AUTHINFO PASS pw\r\n"); err != nil {
		t.Fatalf("user %s: AUTHINFO PASS: %v", user, err)
	}
	line, err := c.ReadLine()
	if err != nil {
		t.Fatalf("user %s: no reply to AUTHINFO PASS: %v (%q)", user, err, line)
	}
	if strings.HasPrefix(line, "281") {
		_, group, _ = c.TryCmd("GROUP e2e.test")
	}
	return line, group
}

// TestDeepRevReaderdefDigitsBareCode: an authentication backend refuses a client with
// "110 NNN text"; the main process gets "NNN text" in the field of the readerdef name. Three
// digits followed by the end, a space or a TAB are such a refusal ("502", "502 Go away",
// "502", TAB, "Go away"); three digits followed by anything else ("100mbit") are a readerdef
// name. When only "NNN " counted, a bare code and a code with a TAB were taken for readerdef
// names that do not exist, and with an admin readerdef DEFAULT that grants read access the
// refused user got 281 and the groups. Each refused user must get a well-formed refusal line
// (the code, a space, a text) and no 281; the user whose entry names readerdef 100mbit must be
// logged in with that readerdef (posting allowed), not refused and not given DEFAULT.
func TestDeepRevReaderdefDigitsBareCode(t *testing.T) {
	s := refusalSite(t, "sp:pw:502 Go away\nbare:pw:502\ntab:pw:502\tGo away\nmb:pw:100mbit\n")
	for _, tc := range []struct{ user, want string }{
		{"sp", "502 Go away"},
		{"bare", "502 Access denied"},
		{"tab", "502 Go away"},
	} {
		line, group := refusalLogin(t, s, tc.user)
		t.Logf("user %s: AUTHINFO PASS -> %q", tc.user, line)
		switch {
		case strings.HasPrefix(line, "281"):
			t.Errorf("user %s, whose file entry is a refusal, was let in: %q; GROUP e2e.test: %q", tc.user, line, group)
		case line != tc.want:
			t.Errorf("user %s: refused with %q, want %q", tc.user, line, tc.want)
		}
	}

	c := s.Reader(t)
	c.ExpectCmd(381, "AUTHINFO USER mb")
	c.ExpectCmd(281, "AUTHINFO PASS pw")
	c.ExpectCmd(200, "MODE READER") // readerdef 100mbit: posting allowed (DEFAULT: 201)
	c.ExpectCmd(211, "GROUP e2e.test")
	c.ExpectCmd(205, "QUIT")
}

// TestDeepRevAuthRefusalBanLine: the "600 <minutes> NNN text" form of a backend refusal (a
// ban, with reader ban lists) sends the client the "NNN text" that follows the minutes. Its
// fields may be separated by spaces or TABs, and a "600" without a code after the minutes, or
// a bare "600", must still give a well-formed refusal line, the generic "502 Access denied",
// not an empty line and not a login.
func TestDeepRevAuthRefusalBanLine(t *testing.T) {
	s := refusalSite(t, "sp:pw:600 1 500 Go away\ntab:pw:600\t1\t500\tGo away\nnocode:pw:600 1\nbare:pw:600\n")
	for _, tc := range []struct{ user, want string }{
		{"sp", "500 Go away"},
		{"tab", "500 Go away"},
		{"nocode", "502 Access denied"},
		{"bare", "502 Access denied"},
	} {
		line, group := refusalLogin(t, s, tc.user)
		t.Logf("user %s: AUTHINFO PASS -> %q", tc.user, line)
		switch {
		case strings.HasPrefix(line, "281"):
			t.Errorf("user %s, whose file entry is a refusal, was let in: %q; GROUP e2e.test: %q", tc.user, line, group)
		case line != tc.want:
			t.Errorf("user %s: refused with %q, want %q", tc.user, line, tc.want)
		}
	}
}

package e2e

import (
	"bytes"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"regexp"
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
	return refusalSiteWith(t, map[string]string{"userpass": userpass}, "file", "userpass")
}

// refusalSiteWith is refusalSite with the backend line "<backend> <s.Dir>/<file>" in the
// authdef (backend "file" or "perl") and files written to the site directory.
func refusalSiteWith(t *testing.T, files map[string]string, backend, file string) *Site {
	t.Helper()
	_, spool := spoolStore(t)
	s := NewReaderSite(t, ReaderOptions{Spool: spool, NoStart: true, Files: files})
	access := `vserverdef DEFAULT
    hostname ` + PathHost + `
end
authdef byfile
    ` + backend + ` ` + filepath.Join(s.Dir, file) + `
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

// refusalLineRE is one well-formed status line and nothing after it.
var refusalLineRE = regexp.MustCompile(`^[1-5][0-9][0-9] [^\r\n\x00]*\r\n$`)

// refusalRaw logs in as user with the password "pw" and returns all bytes the server sends
// after AUTHINFO PASS, until it is quiet for 2 s or closes the connection, and whether it
// closed it. When the reply is 281 it also returns the replies to MODE READER and GROUP.
func refusalRaw(t *testing.T, s *Site, user string) (raw string, closed bool, mode, group string) {
	t.Helper()
	c := s.Reader(t)
	defer c.Close()
	c.ExpectCmd(381, "AUTHINFO USER %s", user)
	if err := c.Send("AUTHINFO PASS pw\r\n"); err != nil {
		t.Fatalf("user %s: AUTHINFO PASS: %v", user, err)
	}
	b, eof := c.ReadUntilQuiet(2*time.Second, 8192)
	raw, closed = string(b), eof
	if strings.HasPrefix(raw, "281") && !closed {
		_, mode, _ = c.TryCmd("MODE READER")
		_, group, _ = c.TryCmd("GROUP e2e.test")
	}
	return raw, closed, mode, group
}

type lineEndCase struct{ user, answer, want string }

// checkLineEndRefusals: each user is refused with exactly the line want plus CRLF and the
// connection is closed; it never gets 281.
func checkLineEndRefusals(t *testing.T, s *Site, cases []lineEndCase) {
	t.Helper()
	for _, tc := range cases {
		raw, closed, mode, group := refusalRaw(t, s, tc.user)
		t.Logf("user %s, answer %q: AUTHINFO PASS -> %q, closed %v", tc.user, tc.answer, raw, closed)
		switch {
		case strings.HasPrefix(raw, "281"):
			t.Errorf("user %s, refused by the backend with %q, was let in: %q; MODE READER: %q; GROUP e2e.test: %q",
				tc.user, tc.answer, raw, mode, group)
		case !refusalLineRE.MatchString(raw):
			t.Errorf("user %s (%q): the reply is not one well-formed status line: %q", tc.user, tc.answer, raw)
		case raw != tc.want+"\r\n":
			t.Errorf("user %s (%q): refused with %q, want %q", tc.user, tc.answer, raw, tc.want+"\r\n")
		case !closed:
			t.Errorf("user %s (%q): the connection is still open after the refusal %q", tc.user, tc.answer, raw)
		}
	}
}

// checkLineEndLogin: user mb, whose backend answer names readerdef 100mbit, logs in with it
// (MODE READER 200: posting allowed; DEFAULT would give 201).
func checkLineEndLogin(t *testing.T, s *Site) {
	t.Helper()
	c := s.Reader(t)
	c.ExpectCmd(381, "AUTHINFO USER mb")
	c.ExpectCmd(281, "AUTHINFO PASS pw")
	c.ExpectCmd(200, "MODE READER")
	c.ExpectCmd(211, "GROUP e2e.test")
	c.ExpectCmd(205, "QUIT")
}

// TestDeepRevReaderdefDigitsLineEndFile: a backend answer can end in a line end. The file
// backend cuts its line at the LF only, so a user file with CRLF gives "502" and a CR. When a
// refusal was three digits followed by the end, a space or a TAB only, "502" and a CR was
// taken for a readerdef name that does not exist, SetAuthDetails() used the first readerdef
// (the admin's DEFAULT, which grants read access), and the refused user got 281. Three digits
// followed by any white space are a refusal; the client gets one well-formed line, then the
// close.
func TestDeepRevReaderdefDigitsLineEndFile(t *testing.T) {
	s := refusalSite(t, "cr:pw:502\r\ncrtxt:pw:502 Go away\r\ncr600:pw:600 1 502 x\r\nmb:pw:100mbit\n")
	checkLineEndRefusals(t, s, []lineEndCase{
		{"cr", "502\r", "502 Access denied"},
		{"crtxt", "502 Go away\r", "502 Go away"},
		{"cr600", "600 1 502 x\r", "502 x"},
	})
	checkLineEndLogin(t, s)
}

// TestDeepRevReaderdefDigitsLineEndPerl: the Perl backend passes the script's answer on as it
// is, so "110 502" with a newline or CRLF reaches the main process as "502" and an LF or CR LF.
// As with the file backend, the refused user got 281 through readerdef DEFAULT. Skipped when
// dreaderd is built without Perl support.
func TestDeepRevReaderdefDigitsLineEndPerl(t *testing.T) {
	b, err := os.ReadFile(filepath.Join(bin(t), "dreaderd"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(b, []byte("PERL auth request")) {
		t.Skip("dreaderd is built without Perl support (PERL_ENABLED)")
	}
	cases := []lineEndCase{
		{"nl", "110 502\n", "502 Access denied"},
		{"crlf", "110 502\r\n", "502 Access denied"},
		{"cr", "110 502\r", "502 Access denied"},
		{"txt", "110 502 Go away\r\n", "502 Go away"},
		{"ban", "110 600 1 502 x\r\n", "502 x"},
	}
	esc := strings.NewReplacer("\r", `\r`, "\n", `\n`)
	var m strings.Builder
	for _, tc := range append(cases, lineEndCase{"mb", "110 100mbit", ""}) {
		fmt.Fprintf(&m, "    \"%s\" => \"%s\",\n", tc.user, esc.Replace(tc.answer))
	}
	script := "my %ans = (\n" + m.String() + ");\n" + `sub checkuser {
    my ($user, $pass) = @_;
    return "0" unless $pass eq "pw" && exists $ans{$user};
    return $ans{$user};
}
1;
`
	s := refusalSiteWith(t, map[string]string{"auth.pl": script}, "perl", "auth.pl")
	for i := range cases {
		cases[i].answer = strings.TrimPrefix(cases[i].answer, "110 ")
	}
	checkLineEndRefusals(t, s, cases)
	checkLineEndLogin(t, s)
}

// TestDeepRevReaderdefDigitsNotARefusalForm: a backend answer "110 NNN-text", "110 NNN,text"
// or "110 NNN:text" is no refusal in the form the main process knows (three digits, then the
// end or white space), and a name such as "123abc" is no readerdef of the site. The main
// process gave all of them to SetAuthDetails() as a readerdef name; a name it does not find
// is replaced there by the first readerdef (the admin's DEFAULT, which grants read access), so
// the user got 281 and the groups. At 4e8828c every name with three leading digits was a
// refusal. A name with three leading digits is refused when no readerdef has it; "100mbit",
// which exists, still logs in with that readerdef. Each refused user gets one well-formed
// line, "502 Access denied", and the close.
func TestDeepRevReaderdefDigitsNotARefusalForm(t *testing.T) {
	s := refusalSite(t, "dash:pw:502-Go away\ncomma:pw:502,x\nunk:pw:123abc\nmb:pw:100mbit\n")
	checkLineEndRefusals(t, s, []lineEndCase{
		{"dash", "502-Go away", "502 Access denied"},
		{"comma", "502,x", "502 Access denied"},
		{"unk", "123abc", "502 Access denied"},
	})
	checkLineEndLogin(t, s)
}

// TestDeepRevReaderdefDigitsNotARefusalFormPerl: the same through the Perl backend, which
// can also answer "502:x" (the file backend ends a name at a colon). Skipped when dreaderd
// is built without Perl support.
func TestDeepRevReaderdefDigitsNotARefusalFormPerl(t *testing.T) {
	b, err := os.ReadFile(filepath.Join(bin(t), "dreaderd"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(b, []byte("PERL auth request")) {
		t.Skip("dreaderd is built without Perl support (PERL_ENABLED)")
	}
	cases := []lineEndCase{
		{"dash", "502-Go away", "502 Access denied"},
		{"colon", "502:x", "502 Access denied"},
		{"comma", "502,x", "502 Access denied"},
		{"unk", "123abc", "502 Access denied"},
	}
	var m strings.Builder
	for _, tc := range append(cases, lineEndCase{"mb", "100mbit", ""}) {
		fmt.Fprintf(&m, "    \"%s\" => \"110 %s\",\n", tc.user, tc.answer)
	}
	script := "my %ans = (\n" + m.String() + ");\n" + `sub checkuser {
    my ($user, $pass) = @_;
    return "0" unless $pass eq "pw" && exists $ans{$user};
    return $ans{$user};
}
1;
`
	s := refusalSiteWith(t, map[string]string{"auth.pl": script}, "perl", "auth.pl")
	checkLineEndRefusals(t, s, cases)
	checkLineEndLogin(t, s)
}

package e2e

import (
	"path/filepath"
	"strings"
	"testing"
)

// TestAuthRefusalFullCode: an authentication backend refuses a client with "110 NNN text".
// The main process checked only that NNN were three digits and sent any of them on, so a
// backend answer "110 281 Fine" or "110 200 hi" reached the client after AUTHINFO PASS as a
// success code, on a connection that is refused and closed. The code is now compared as the
// whole number: 400, 481 (after AUTHINFO) and 502 go out as they are, any other code becomes
// 502, and the backend's text is kept.
func TestAuthRefusalFullCode(t *testing.T) {
	s := refusalSite(t, "ok:pw:281 Fine\nsrv:pw:200 hi\nunk:pw:999 x\n"+
		"bad:pw:481 Bad password\nlater:pw:400 Try later\nno:pw:502 No\n")
	for _, tc := range []struct{ user, want string }{
		{"ok", "502 Fine"},
		{"srv", "502 hi"},
		{"unk", "502 x"},
		{"bad", "481 Bad password"},
		{"later", "400 Try later"},
		{"no", "502 No"},
	} {
		line, group := refusalLogin(t, s, tc.user)
		t.Logf("user %s: AUTHINFO PASS -> %q", tc.user, line)
		if line != tc.want {
			t.Errorf("user %s: refused with %q, want %q (GROUP e2e.test: %q)", tc.user, line, tc.want, group)
		}
	}
}

// TestAuthFailedNoStatusLine: a client whose AUTHINFO PASS is wrong gets no readerdef name
// from the worker, and the main process takes the first readerdef of dreader.access for it.
// When that readerdef asks for the status report ("status yes", a monitoring entry), the
// refused client got the "180 ... status: connect= ..." line with the server's counters
// instead of 481. The status line is now only for a client that is let in.
func TestAuthFailedNoStatusLine(t *testing.T) {
	_, spool := spoolStore(t)
	s := NewReaderSite(t, ReaderOptions{Spool: spool, NoStart: true,
		Files: map[string]string{"userpass": "fred:fredspass\n"}})
	s.WriteFile(t, "dreader.access", `vserverdef DEFAULT
    hostname `+PathHost+`
end
authdef byfile
    file `+filepath.Join(s.Dir, "userpass")+`
end
readerdef monitor
    status yes
end
readerdef userfile
    read yes
    auth byfile
end
access 127.0.0.1 userfile
`)
	startReader(t, s, "[127.0.0.1]:"+s.Addr[strings.LastIndex(s.Addr, ":")+1:])

	c := s.Reader(t)
	defer c.Close()
	c.ExpectCmd(381, "AUTHINFO USER fred")
	if err := c.Send("AUTHINFO PASS wrong\r\n"); err != nil {
		t.Fatalf("AUTHINFO PASS: %v", err)
	}
	line, err := c.ReadLine()
	if err != nil {
		t.Fatalf("no reply to AUTHINFO PASS: %v (%q)", err, line)
	}
	if !strings.HasPrefix(line, "481 ") {
		t.Errorf("wrong password: %q, want 481", line)
	}

	// the right password still logs in
	c2 := s.Reader(t)
	defer c2.Close()
	c2.ExpectCmd(381, "AUTHINFO USER fred")
	c2.ExpectCmd(281, "AUTHINFO PASS fredspass")
}

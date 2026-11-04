package e2e

import (
	"os"
	"path/filepath"
	"testing"
)

// TestReaderAuthinfoAfterLogin (D-123, RFC 4643 section 2.2): after a successful login (281),
// AUTHINFO answers 502; a connection allowed by its address alone has not logged in and may
// still use AUTHINFO. dreaderd now keeps the login as a flag set with the 281. Before, it took
// "logged in" from a non-empty stored password, so a login without a password (an authdef's
// ident + user match, which this test cannot set up without an ident server) could log in
// again.
func TestReaderAuthinfoAfterLogin(t *testing.T) {
	users := filepath.Join(t.TempDir(), "users")
	if err := os.WriteFile(users, []byte("bob:secret\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	access := `vserverdef DEFAULT
    hostname ` + PathHost + `
end
authdef byfile
    file ` + users + `
end
readerdef needauth
    read yes
    auth byfile
end
readerdef open
    read yes
end
access 127.0.0.1 needauth
access ` + PeerIP + ` open
`
	_, spool := spoolStore(t)
	s := NewReaderSite(t, ReaderOptions{Spool: spool, Access: access})

	c := s.Reader(t)
	c.ExpectCmd(381, "AUTHINFO USER bob")
	c.ExpectCmd(281, "AUTHINFO PASS secret")
	c.ExpectCmd(502, "AUTHINFO USER mallory")
	c.ExpectCmd(502, "AUTHINFO PASS other")
	c.ExpectCmd(205, "QUIT")

	// allowed by address: AUTHINFO is still open to it
	o := s.dialFrom(t, PeerIP)
	o.ExpectCmd(381, "AUTHINFO USER bob")
	o.ExpectCmd(205, "QUIT")
}

// TestReaderCapabilitiesArgument (D-124, RFC 3977 section 5.2.2): CAPABILITIES with a keyword
// the server does not know answers 101 as without one; an argument that is not a keyword
// (ALPHA 2*(ALPHA / DIGIT / "." / "-")), or more than one, answers 501. Before, every
// argument was ignored.
func TestReaderCapabilitiesArgument(t *testing.T) {
	_, spool := spoolStore(t)
	s := NewReaderSite(t, ReaderOptions{Spool: spool})
	c := s.Reader(t)
	for _, arg := range []string{"", "AUTOUPDATE", "x-y.z"} {
		code, line := c.Cmd("CAPABILITIES %s", arg)
		if code != 101 {
			t.Errorf("CAPABILITIES %s: %q, want 101", arg, line)
			continue
		}
		c.ReadDotLines()
	}
	for _, arg := range []string{"1x", "ab", "a!b", "foo bar"} {
		if code, line := c.Cmd("CAPABILITIES %s", arg); code != 501 {
			t.Errorf("CAPABILITIES %s: %q, want 501", arg, line)
			if code == 101 {
				c.ReadDotLines()
			}
		}
	}
	c.ExpectCmd(205, "QUIT")
}

// TestReaderUnavailableCommand502 (D-125, RFC 3977 section 3.2.1): a command dreaderd has but
// this connection may not use answers 502: TAKETHIS on a reader connection, GROUP on a
// feed-only connection. On a feed connection that may switch with MODE READER, GROUP stays
// 500 (502 must not mean "available after MODE READER"). An unknown command is 500 everywhere.
// Before, all of them answered 500.
func TestReaderUnavailableCommand502(t *testing.T) {
	_, spool := spoolStore(t)
	s := NewReaderSite(t, ReaderOptions{Spool: spool})
	r := s.Reader(t)
	r.ExpectCmd(502, "TAKETHIS <d125@e2e.test>")
	r.ExpectCmd(500, "FROBNICATE")
	r.ExpectCmd(205, "QUIT")

	f := s.dialFrom(t, PeerIP) // DefaultReaderAccess: feed yes only
	f.ExpectCmd(502, "GROUP e2e.test")
	f.ExpectCmd(500, "FROBNICATE")
	f.ExpectCmd(205, "QUIT")

	access := `vserverdef DEFAULT
    hostname ` + PathHost + `
end
readerdef rp
    read yes
end
readerdef feedread
    feed yes
    read yes
end
access 127.0.0.1 rp
access ` + PeerIP + ` feedread
`
	_, spool2 := spoolStore(t)
	s2 := NewReaderSite(t, ReaderOptions{Spool: spool2, Access: access})
	m := s2.dialFrom(t, PeerIP)
	m.ExpectCmd(500, "GROUP e2e.test")
	m.ExpectCmd(205, "QUIT")
}

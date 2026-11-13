package e2e

import (
	"bufio"
	"net"
	"strconv"
	"sync/atomic"
	"testing"

	"github.com/go-while/nntp-testkit/client"
)

// deepRevLDAPServer answers a bind (success), a search (one entry) and a compare with the
// result code compare (6 = compareTrue, 5 = compareFalse), and counts the searches.
func deepRevLDAPServer(t *testing.T, compare byte) (int, *atomic.Int32) {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { l.Close() })
	searches := new(atomic.Int32)
	result := func(code byte) []byte {
		r := ldapTLV(0x0a, []byte{code})
		r = append(r, ldapTLV(0x04, nil)...)
		return append(r, ldapTLV(0x04, nil)...)
	}
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				r := bufio.NewReader(c)
				for {
					_, msg, err := ldapReadTLV(r)
					if err != nil || len(msg) < 3 || msg[0] != 0x02 || len(msg) < 3+int(msg[1]) {
						return
					}
					idTLV := ldapTLV(0x02, msg[2:2+int(msg[1])])
					var out []byte
					switch msg[2+int(msg[1])] {
					case 0x60: // bind
						out = ldapTLV(0x30, append(idTLV, ldapTLV(0x61, result(0))...))
					case 0x63: // search: one entry, then done
						searches.Add(1)
						vals := ldapTLV(0x31, ldapTLV(0x04, []byte("pw")))
						attr := ldapTLV(0x30, append(ldapTLV(0x04, []byte("userPassword")), vals...))
						entry := ldapTLV(0x64, append(ldapTLV(0x04, []byte("uid=x,dc=e2e,dc=test")), ldapTLV(0x30, attr)...))
						out = ldapTLV(0x30, append(idTLV, entry...))
						out = append(out, ldapTLV(0x30, append(idTLV, ldapTLV(0x65, result(0))...))...)
					case 0x6e: // compare
						out = ldapTLV(0x30, append(idTLV, ldapTLV(0x6f, result(compare))...))
					default:
						continue
					}
					if _, err := c.Write(out); err != nil {
						return
					}
				}
			}(c)
		}
	}()
	return l.Addr().(*net.TCPAddr).Port, searches
}

// TestDeepRevLDAPTwoAuthdefs (L6-3): the default LDAP code of dreaderd keeps one connection
// per DNS worker between logins (dreaderd/dns.c, LDAPAuthenticate()), and with it the
// parsed URL of the authdef that opened it: server, base, scope and filter. A login through
// another ldap authdef used that connection and URL as they were, so the directory of the
// authdef the worker happened to see first answered for every other one.
//
// Two authdefs with a directory each: directory A knows the password, directory B does
// not. After a login through A, the same user and password through B must be refused, by
// B, and a login through A must still work afterwards.
func TestDeepRevLDAPTwoAuthdefs(t *testing.T) {
	builtWith(t, "LDAP bind failed", "LDAP support (LDAP_ENABLED)")
	portA, searchesA := deepRevLDAPServer(t, 6)
	portB, searchesB := deepRevLDAPServer(t, 5)
	url := func(port int) string {
		return "ldap ldap://127.0.0.1:" + strconv.Itoa(port) + "/dc=e2e,dc=test?userPassword?sub?(uid=%25s)"
	}
	access := `vserverdef DEFAULT
    hostname ` + PathHost + `
end
authdef la
    realm a
    ` + url(portA) + `
end
authdef lb
    realm b
    ` + url(portB) + `
end
readerdef ra
    read yes
    auth la
end
readerdef rb
    read yes
    auth lb
end
access 127.0.0.1 ra
access 127.0.0.1 rb
`
	s := authTimeoutSite(t, access)
	login := func(user string) (int, string) {
		t.Helper()
		c := client.DialWith(t, s.Addr, 2*authTimeoutBound)
		defer c.Close()
		c.ExpectCmd(381, "AUTHINFO USER %s", user)
		return c.Cmd("AUTHINFO PASS pw")
	}

	if code, line := login("x@a"); code != 281 {
		t.Fatalf("x@a (directory A accepts the password): %q, want 281", line)
	}
	if code, line := login("x@b"); code == 281 {
		t.Errorf("x@b logged in (%q) although directory B refuses the password: the login was checked against directory A (searches: A %d, B %d)",
			line, searchesA.Load(), searchesB.Load())
	}
	if n := searchesB.Load(); n != 1 {
		t.Errorf("directory B was searched %d times for the login x@b, want 1 (directory A: %d)", n, searchesA.Load())
	}
	if code, line := login("x@a"); code != 281 {
		t.Errorf("x@a after a login through the other authdef: %q, want 281", line)
	}
}

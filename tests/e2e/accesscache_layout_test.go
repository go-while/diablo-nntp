package e2e

import (
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	testkit "github.com/go-while/nntp-testkit"
)

// authDefSize is sizeof(AuthDef) in dreaderd/defs.h: au_Name[128], 13 fields of 255 bytes
// (one of them the unused au_LDAP slot) and an int, aligned to 4.
const authDefSize = 128 + 13*255 + 1 + 4

// TestAccessCacheAuthDefLayout: cache.authdef is an array of AuthDef records, read back with
// the size of the running binary and no format check. A cache that an older dreaderd wrote
// is read when the start-up rebuild fails (an option in dreader.access that is no longer
// valid, an I/O error): with a smaller record the names are read at the wrong offsets, a
// readerdef's authdef is not found, and DEFAULT (no authentication) is used, so a reader
// that had to log in gets read access without AUTHINFO. Removing au_LDAP (#402) shrank the
// record from 3448 to 3192 bytes; the slot stays as padding. The file holds the implicit
// DEFAULT and the one authdef of this dreader.access.
func TestAccessCacheAuthDefLayout(t *testing.T) {
	_, spool := spoolStore(t)
	s := NewReaderSite(t, ReaderOptions{Spool: spool, NoStart: true,
		Files: map[string]string{"userpass": "fred:fredspass\n"}})
	s.WriteFile(t, "dreader.access", `vserverdef DEFAULT
    hostname `+PathHost+`
end
authdef byfile
    file `+filepath.Join(s.Dir, "userpass")+`
end
readerdef userfile
    read yes
    auth byfile
end
access 127.0.0.1 userfile
`)
	_, port, _ := net.SplitHostPort(s.Addr)
	startReader(t, s, "[127.0.0.1]:"+port)

	st, err := os.Stat(filepath.Join(s.Dir, "cache.authdef"))
	if err != nil {
		t.Fatal(err)
	}
	if st.Size() != 2*authDefSize {
		t.Errorf("cache.authdef is %d bytes, want %d (2 AuthDef records of %d bytes): the AuthDef layout changed, so a cache from an older dreaderd would be misread",
			st.Size(), 2*authDefSize, authDefSize)
	}

	// the authdef is still in force: GROUP needs a login first, which works
	c := s.Reader(t)
	defer c.Close()
	c.ExpectCmd(480, "GROUP e2e.test")
	c.ExpectCmd(381, "AUTHINFO USER fred")
	c.ExpectCmd(281, "AUTHINFO PASS fredspass")
}

// TestAccessCacheOldLDAPAuthdefRefused: an AuthDef that an older dreaderd cached with only
// au_LDAP set (an authdef "ldap <url>") must still need a login, and the login must be refused:
// LDAP is gone. With nothing reading au_LDAP, AuthNeeded stayed 0 and such a reader got read
// access without AUTHINFO. The test writes that record into the running site's cache.authdef
// (the file backend's path cleared, au_LDAP set), makes the processes load the caches again
// (a newer .accesslock, as an install by another process would), and checks 480 before the
// login and a refusal for the login.
func TestAccessCacheOldLDAPAuthdefRefused(t *testing.T) {
	_, spool := spoolStore(t)
	s := NewReaderSite(t, ReaderOptions{Spool: spool, NoStart: true,
		Files: map[string]string{"userpass": "fred:fredspass\n"}})
	s.WriteFile(t, "dreader.access", `vserverdef DEFAULT
    hostname `+PathHost+`
end
authdef byfile
    file `+filepath.Join(s.Dir, "userpass")+`
end
readerdef userfile
    read yes
    auth byfile
end
access 127.0.0.1 userfile
`)
	_, port, _ := net.SplitHostPort(s.Addr)
	startReader(t, s, "[127.0.0.1]:"+port)

	// record 1 is authdef byfile: clear au_File, set au_LDAP (offsets in AuthDef)
	const nameOff, fileOff, ldapOff = 0, 128, 128 + 10*255
	path := filepath.Join(s.Dir, "cache.authdef")
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(b) != 2*authDefSize {
		t.Fatalf("cache.authdef is %d bytes, want %d", len(b), 2*authDefSize)
	}
	rec := b[authDefSize : 2*authDefSize]
	if name := string(rec[nameOff : nameOff+6]); name != "byfile" {
		t.Fatalf("record 1 of cache.authdef is %q, want byfile", name)
	}
	for i := fileOff; i < fileOff+255; i++ {
		rec[i] = 0
	}
	copy(rec[ldapOff:ldapOff+254], "ldap://127.0.0.1/ou=users\x00")
	if err := os.WriteFile(path, b, 0o600); err != nil {
		t.Fatal(err)
	}
	future := time.Now().Add(time.Hour)
	if err := os.Chtimes(filepath.Join(s.Dir, ".accesslock"), future, future); err != nil {
		t.Fatal(err)
	}

	// once the caches are loaded again, GROUP needs a login
	var code int
	testkit.WaitFor(t, 10*time.Second, "480 for GROUP with the old LDAP authdef", func() bool {
		c := s.Reader(t)
		defer c.Close()
		code, _, _ = c.TryCmd("GROUP e2e.test")
		return code == 480
	})
	c := s.Reader(t)
	defer c.Close()
	c.ExpectCmd(381, "AUTHINFO USER fred")
	if code, line, err := c.TryCmd("AUTHINFO PASS fredspass"); err == nil && code == 281 {
		t.Errorf("the login against an authdef that only names LDAP was accepted: %q", line)
	}
	if !strings.Contains(generalLog(s), "LDAP authentication requested, but no longer supported") {
		t.Errorf("no refusal of the LDAP authdef in the log:\n%s", generalLog(s))
	}
}

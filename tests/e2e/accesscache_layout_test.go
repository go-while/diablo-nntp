package e2e

import (
	"net"
	"os"
	"path/filepath"
	"testing"
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

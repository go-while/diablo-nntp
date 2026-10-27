package e2e

import (
	"fmt"
	"net"
	"path/filepath"
	"testing"
	"time"
)

// TestReaderFileAuthRejectString: a file auth entry whose third field is a reject string
// ("600 <minutes> 500 <text>") turns the login away with that reply, and the dreaderd master
// survives it, login after login. Before the fix the master logged the rejection through the
// vserver pointer of the DNS worker's reply, which is only valid in the master after
// SetAuthDetails() (the reject path skips it). dreader.access is changed before each
// AUTHINFO PASS: the worker then maps the rebuilt access cache at an address the master has
// not mapped, and the master died with SIGSEGV, ending all reader service.
func TestReaderFileAuthRejectString(t *testing.T) {
	_, spool := spoolStore(t)
	s := NewReaderSite(t, ReaderOptions{Spool: spool, NoStart: true,
		Files: map[string]string{"userpass": "fred:fredspass\nbanned:pw:600 1 500 You are banned\n"}})
	access := `vserverdef DEFAULT
    hostname ` + PathHost + `
end
authdef byfile
    file ` + filepath.Join(s.Dir, "userpass") + `
end
readerdef userfile
    read yes
    auth byfile
end
access 127.0.0.1 userfile
`
	s.WriteFile(t, "dreader.access", access)
	_, port, err := net.SplitHostPort(s.Addr)
	if err != nil {
		t.Fatal(err)
	}
	startReader(t, s, "[127.0.0.1]:"+port)

	for i := 0; i < 3; i++ {
		c := s.Reader(t)
		expect(t, c, 381, "AUTHINFO USER banned")
		// Change dreader.access between the greeting (whose access check remaps the cache
		// in the master too) and the login, so only the DNS worker maps the rebuilt cache.
		// The cache's lock file has a one-second timestamp: wait for the next second.
		time.Sleep(1100 * time.Millisecond)
		s.WriteFile(t, "dreader.access", access+fmt.Sprintf("# change %d\n", i))
		expect(t, c, 500, "AUTHINFO PASS pw")
		c.Close()
	}

	// The master is still there and serves a good login.
	c := s.Reader(t)
	expect(t, c, 381, "AUTHINFO USER fred")
	expect(t, c, 281, "AUTHINFO PASS fredspass")
	expect(t, c, 205, "QUIT")
}

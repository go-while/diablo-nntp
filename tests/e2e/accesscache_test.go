package e2e

// dreaderd's compiled dreader.access caches and group-name checks.

import (
	"net"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

// readerAuthAccess lets 127.0.0.1 read after AUTHINFO as user fred.
const readerAuthAccess = `vserverdef DEFAULT
    hostname ` + PathHost + `
end
authdef fred
    user fred
    pass fredspass
end
readerdef userfred
    read yes
    post yes
    auth fred
end
readerdef feed
    feed yes
end
access 127.0.0.1 userfred
access ` + PeerIP + ` feed
`

var accessCaches = []string{"cache.vserverdef", "cache.groupdef", "cache.authdef",
	"cache.readerdef", "cache.readeraccess"}

// TestReaderAccessCachePrivate: the caches dreaderd compiles from dreader.access (one of
// them holds the passwords) are created mode 0600 whatever the umask, and caches left
// from an older version with a wider mode are replaced with 0600 ones.
func TestReaderAccessCachePrivate(t *testing.T) {
	old := syscall.Umask(0o022) // the usual umask of a daemon started by root
	t.Cleanup(func() { syscall.Umask(old) })

	_, spool := spoolStore(t)
	s := NewReaderSite(t, ReaderOptions{Spool: spool, Access: readerAuthAccess, NoStart: true})
	// caches (and a stale .new) as an older dreaderd left them
	for _, name := range []string{"cache.authdef", "cache.authdef.new", "cache.groupdef"} {
		p := filepath.Join(s.Dir, name)
		if err := os.WriteFile(p, []byte("old"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(p, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	_, port, _ := net.SplitHostPort(s.Addr)
	startReader(t, s, "[127.0.0.1]:"+port)

	c := s.Reader(t)
	expect(t, c, 381, "AUTHINFO USER fred")
	expect(t, c, 281, "AUTHINFO PASS fredspass")
	expect(t, c, 211, "GROUP e2e.test")
	expect(t, c, 205, "QUIT")

	for _, name := range accessCaches {
		fi, err := os.Stat(filepath.Join(s.Dir, name))
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		if m := fi.Mode().Perm(); m&0o077 != 0 {
			t.Errorf("%s has mode %#o, want 0600 (umask 022)", name, m)
		}
	}
}

// TestGroupNameSlashRefused: a group name containing '/' is not valid (with
// readergrouphash hierarchy group names become path components); a name with the
// other allowed characters still is.
func TestGroupNameSlashRefused(t *testing.T) {
	_, spool := spoolStore(t)
	s := NewReaderSite(t, ReaderOptions{Spool: spool, NoStart: true})

	out, _ := s.TryRun("dgrpctl", "newgroup", "e2e.with/slash", "y")
	if !strings.Contains(out, "Illegal newsgroup name") {
		t.Errorf("dgrpctl newgroup e2e.with/slash: %q, want Illegal newsgroup name", out)
	}
	s.Run(t, "dgrpctl", "newgroup", "e2e.plus+minus-9", "y")

	active := s.Run(t, "dgrpctl", "listactive")
	if strings.Contains(active, "slash") {
		t.Errorf("e2e.with/slash was created:\n%s", active)
	}
	if !strings.Contains(active, "e2e.plus+minus-9") {
		t.Errorf("e2e.plus+minus-9 missing:\n%s", active)
	}
}

// TestGroupNameTooLongRefused: a group name of MAXGNAME (256) characters or more is not
// valid; one of 255 characters still is.
func TestGroupNameTooLongRefused(t *testing.T) {
	_, spool := spoolStore(t)
	s := NewReaderSite(t, ReaderOptions{Spool: spool, NoStart: true})

	long := "e2e." + strings.Repeat("a", 300)
	out, _ := s.TryRun("dgrpctl", "newgroup", long, "y")
	if !strings.Contains(out, "Illegal newsgroup name") {
		t.Errorf("dgrpctl newgroup with a 304-character name: %q, want Illegal newsgroup name", out)
	}
	ok := "e2e." + strings.Repeat("b", 251)
	s.Run(t, "dgrpctl", "newgroup", ok, "y")

	active := s.Run(t, "dgrpctl", "listactive")
	if strings.Contains(active, long) {
		t.Errorf("the 304-character name was created")
	}
	if !strings.Contains(active, ok) {
		t.Errorf("the 255-character name is missing from listactive")
	}
}

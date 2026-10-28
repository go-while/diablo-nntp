package e2e

// File modes: what the daemons create that holds private data is not readable by other users,
// whatever the umask, and the install script keeps the log and run directories to the news user
// and group.

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

// withUmask0 runs the daemons started by f with umask 0, so that only a mode a program sets
// itself can keep a file private.
func withUmask0(f func()) {
	old := syscall.Umask(0)
	defer syscall.Umask(old)
	f()
}

// secretsAccess is a dreader.access with the AUTHINFO password, cryptpw and Cancel-Lock secret
// that the cache files copy.
func secretsAccess() string {
	return strings.Replace(AuthReaderAccess, "    hostname "+PathHost+"\n",
		"    hostname "+PathHost+"\n    cryptpw s3cretCrypt\n    canlockadmin e2e-modes-secret\n", 1)
}

// TestCacheFilesPrivate: dreaderd's cache files (they hold passwords and secrets of
// dreader.access), its status file and its logs are not readable by others, with umask 0.
func TestCacheFilesPrivate(t *testing.T) {
	var s *Site
	withUmask0(func() {
		_, spool := spoolStore(t)
		s = NewReaderSite(t, ReaderOptions{Spool: spool, Access: secretsAccess()})
		c := s.Reader(t)
		expect(t, c, 381, "AUTHINFO USER fred")
		expect(t, c, 281, "AUTHINFO PASS fredspass")
		expect(t, c, 205, "QUIT")
	})
	seen := map[string]bool{}
	err := filepath.WalkDir(s.Dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(s.Dir, path)
		fi, err := d.Info()
		if err != nil {
			return err
		}
		perm := fi.Mode().Perm()
		base := filepath.Base(rel)
		switch {
		case strings.HasPrefix(base, "cache.") || base == ".accesslock":
			seen["cache"] = true
			if perm&0o077 != 0 {
				t.Errorf("%s: mode %04o, want no access for group and others", rel, perm)
			}
		case strings.HasPrefix(rel, "log"+string(filepath.Separator)) && base != "dreaderd.pid":
			seen["log"] = true
			if perm&0o007 != 0 || perm&0o020 != 0 {
				t.Errorf("%s: mode %04o, want no access for others and no write for the group", rel, perm)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !seen["cache"] {
		t.Error("no cache files found")
	}
	// the cache files do hold the secrets (that is why they are private)
	b, err := os.ReadFile(filepath.Join(s.Dir, "cache.authdef"))
	if err != nil || !strings.Contains(string(b), "fredspass") {
		t.Errorf("cache.authdef: %v; expected it to hold the AUTHINFO password", err)
	}
}

// TestFeederLogsPrivate: the logs and the status file of a feeder.
func TestFeederLogsPrivate(t *testing.T) {
	var s *Site
	withUmask0(func() {
		s = NewSite(t, SiteOptions{})
		p := s.Peer(t)
		expect(t, p, 205, "QUIT")
	})
	logs, _ := filepath.Glob(filepath.Join(s.Dir, "log", "*"))
	for _, p := range logs {
		if filepath.Base(p) == "diablo.pid" {
			continue
		}
		fi, err := os.Stat(p)
		if err != nil || fi.IsDir() {
			continue
		}
		if perm := fi.Mode().Perm(); perm&0o027 != 0 {
			t.Errorf("%s: mode %04o, want no write for the group and no access for others", filepath.Base(p), perm)
		}
	}
}

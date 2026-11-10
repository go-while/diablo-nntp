package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestDeepRevAccessCacheRenameFailureLogged (L6-7): dreaderd writes the five access cache
// files as <name>.new and renames them into place (dreaderd/dns.c, RenameAccessCache()).
// The result of rename() was not looked at: when one of the five could not be renamed, the
// cache stayed a mix of new and old files (or lacked one) and nothing was logged, nor was
// the install tried again.
//
// cache.readeraccess is a directory that is not empty, so it cannot be replaced: dreaderd must
// say so in its log.
func TestDeepRevAccessCacheRenameFailureLogged(t *testing.T) {
	_, spool := spoolStore(t)
	s := NewReaderSite(t, ReaderOptions{Spool: spool, NoStart: true})
	dir := filepath.Join(s.Dir, "cache.readeraccess")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "keep"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	startReaderNoWait(t, s)

	// the directory cannot get the hard link that would put it back, so the install stops
	// there (before any rename) or, where a directory can be linked, at its rename
	logged := func(log string) bool {
		return strings.Contains(log, "Unable to link "+dir+" ") || strings.Contains(log, "Unable to rename "+dir+".new")
	}
	limit := time.Now().Add(15 * time.Second)
	for time.Now().Before(limit) && !logged(generalLog(s)) {
		time.Sleep(200 * time.Millisecond)
	}
	if log := generalLog(s); !logged(log) {
		t.Errorf("general.log does not say that %s could not be replaced: the failed install of an access cache file went unnoticed\n%s", dir, log)
	}
}

package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestDexpirecacheDefaultDir: without -d, dexpirecache works on the configured cache
// directory (path_cache) and reads diablo.config given with -C. Before, -d had no default
// (chdir(NULL): "Unable to change to `(null)'") and -C was an unknown option.
func TestDexpirecacheDefaultDir(t *testing.T) {
	s := NewSite(t, SiteOptions{NoStart: true})
	if err := os.MkdirAll(filepath.Join(s.Dir, "spool/cache"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"-v", "-f", "1"},
		{"-v", "-f", "1", "-C", filepath.Join(s.Dir, "diablo.config")},
	} {
		out, err := s.TryRun("dexpirecache", args...)
		if err != nil || !strings.Contains(out, "No need to expire cache") {
			t.Errorf("dexpirecache %s: %v\n%s", strings.Join(args, " "), err, out)
		}
	}
}

package e2e

import (
	"strings"
	"testing"
)

// TestDeepRevReaderCacheDirsRefused (L2-14): "drcmd config readercachedirs <value>" with a
// value that is refused (a level below 1, or not a number) answered with the error but had
// already changed the setting of the running dreaderd: SetCacheDirs() wrote each level into
// the live structure before it checked the next one. "512/0" left one level (512), "abc"
// left none, and the cache files written before were no longer found. A refused value must
// leave the setting as it was.
func TestDeepRevReaderCacheDirsRefused(t *testing.T) {
	_, spool := spoolStore(t)
	s := NewReaderSite(t, ReaderOptions{Spool: spool})

	const good = "readercachedirs: 64/32\n"
	s.Run(t, "drcmd", "config", "readercachedirs", "64/32")
	if out := s.Run(t, "drcmd", "config", "readercachedirs"); !strings.Contains(out, good) {
		t.Fatalf("after drcmd config readercachedirs 64/32: %q, want %q", out, good)
	}
	for _, bad := range []string{"512/0", "abc", "16/8/x"} {
		out, _ := s.TryRun("drcmd", "config", "readercachedirs", bad)
		if !strings.Contains(out, "Illegal reader cache dir format") {
			t.Errorf("drcmd config readercachedirs %s was not refused: %q", bad, out)
		}
		if out := s.Run(t, "drcmd", "config", "readercachedirs"); !strings.Contains(out, good) {
			t.Errorf("after the refused readercachedirs %s the setting is %q, want it unchanged: %q", bad, out, good)
		}
	}
}

package e2e

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestDeepRevDexpireStrayDEntry (L7-02): dexpire takes every name in a spool directory that
// starts with "D." as an article directory named after its creation time in hex minutes, and
// does not look at the rest of the name. A stray entry such as "D.tmp" has no hex digits, so
// its time is 0 and the spool looked 50 years old: with "keeptime" set, each pass removed the
// oldest real directory although it was not due, the passes repeat while something was
// removed, and the newest real directory was not protected either, because the stray name
// sorts after it and counted as the newer one. One run removed every article directory.
//
// A spool with keeptime 60 minutes, directories 180, 42 and 12 minutes old and a file
// "D.tmp": only the 180-minute directory may go.
func TestDeepRevDexpireStrayDEntry(t *testing.T) {
	now := time.Now().Unix() / 60
	type dir struct {
		ageM int64
		old  bool // older than keeptime, may go
	}
	dirs := []dir{{180, true}, {42, false}, {12, false}}
	path := func(s *Site, d dir) string {
		return filepath.Join(s.Dir, "spool", "news", "P.01", fmt.Sprintf("D.%08x", now-d.ageM))
	}
	s := NewSite(t, SiteOptions{
		NoStart: true,
		SpoolCtl: `spool 01
    keeptime 3600
end
metaspool m
    spool 01
end
expire * m
`,
		Setup: func(tb testing.TB, s *Site) {
			for _, d := range dirs {
				if err := os.MkdirAll(path(s, d), 0o755); err != nil {
					tb.Fatal(err)
				}
			}
			stray := filepath.Join(s.Dir, "spool", "news", "P.01", "D.tmp")
			if err := os.WriteFile(stray, []byte("not a spool directory\n"), 0o644); err != nil {
				tb.Fatal(err)
			}
		},
	})
	// dexpire sleeps 10 s between passes: up to four passes while the bug is there.
	out := s.Run(t, "dexpire", "-a", "-h0", "-v")
	for _, d := range dirs {
		_, err := os.Stat(path(s, d))
		gone := err != nil
		switch {
		case d.old && !gone:
			t.Errorf("the directory %d min old (keeptime 60) was not removed", d.ageM)
		case !d.old && gone:
			t.Errorf("the directory %d min old (keeptime 60) was removed", d.ageM)
		}
	}
	if t.Failed() {
		t.Logf("dexpire -a -h0 -v:\n%s", out)
	}
}

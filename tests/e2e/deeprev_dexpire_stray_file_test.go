package e2e

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestDeepRevDexpireStrayDFile: dexpire never removes the newest directory of a spool object,
// because diablo is writing into it. It finds "the newest" by name, among the entries called
// D.<8 hex digits>, and did not look at what the entry is: a regular file with such a name
// that sorts after the real directories (D.ffffffff, left by an admin or a recovery) counted
// as the newest directory, and while a size or space limit was not met the newest real
// directory was removed.
//
// A spool with maxsize 1k, two directories of 64 KB each (30 and 5 minutes old) and a file
// D.ffffffff: the limit cannot be met, the older directory goes, the newer one must stay.
func TestDeepRevDexpireStrayDFile(t *testing.T) {
	now := time.Now().Unix() / 60
	var spool, older, newest, stray string
	s := NewSite(t, SiteOptions{
		NoStart: true,
		SpoolCtl: `spool 01
    maxsize 1k
end
metaspool m
    spool 01
end
expire * m
`,
		Setup: func(tb testing.TB, s *Site) {
			spool = filepath.Join(s.Dir, "spool", "news", "P.01")
			older = filepath.Join(spool, fmt.Sprintf("D.%08x", now-30))
			newest = filepath.Join(spool, fmt.Sprintf("D.%08x", now-5))
			stray = filepath.Join(spool, "D.ffffffff")
			for _, d := range []string{older, newest} {
				if err := os.MkdirAll(d, 0o755); err != nil {
					tb.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(d, "B.0001"), bytes.Repeat([]byte("x"), 64<<10), 0o644); err != nil {
					tb.Fatal(err)
				}
			}
			if err := os.WriteFile(stray, []byte("not a spool directory\n"), 0o644); err != nil {
				tb.Fatal(err)
			}
		},
	})
	out := s.Run(t, "dexpire", "-a", "-h0", "-v")
	if _, err := os.Stat(newest); err != nil {
		t.Errorf("the newest directory of the spool was removed: %v", err)
	}
	if _, err := os.Stat(older); err == nil {
		t.Errorf("the older directory was not removed although the spool is over maxsize")
	}
	if _, err := os.Stat(stray); err != nil {
		t.Errorf("the stray file D.ffffffff is gone: %v", err)
	}
	if !strings.Contains(out, "not a directory") {
		t.Errorf("dexpire did not report the stray file")
	}
	if t.Failed() {
		t.Logf("dexpire -a -h0 -v:\n%s", out)
	}
}

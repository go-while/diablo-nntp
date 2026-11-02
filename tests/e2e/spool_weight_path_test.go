package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestSpoolDefaultWeightPath: a spool in dspool.ctl without "weight" gets the size of its file system as
// its weight. addSpool() asked statfs about the spool's path as written in dspool.ctl (P.01), relative to
// the working directory of the program. dexpire reads dspool.ctl before it changes to the spool directory
// (diablo changes to it first, and dreaderd does not read dspool.ctl), so dexpire logged "unable to statfs
// P.01 (No such file or directory)" for each spool, at every run, and the weight was 1 instead.
func TestSpoolDefaultWeightPath(t *testing.T) {
	s := NewSite(t, SiteOptions{
		NoStart: true,
		Config:  map[string]string{"path_generallog": "%s/general.log"},
		SpoolCtl: `spool 01
    maxsize	50g
    keeptime	30d
end
spool 02
    minfree	150g
end
metaspool binaries
    spool       02
    arttypes	binary
end
metaspool nonbinaries
    spool	01
    arttypes	!binary
end
expire * nonbinaries
expire * binaries
`,
		Setup: func(tb testing.TB, s *Site) {
			for _, d := range []string{"P.01", "P.02"} {
				if err := os.MkdirAll(filepath.Join(s.Dir, "spool", "news", d), 0o755); err != nil {
					tb.Fatal(err)
				}
			}
		},
	})
	out := s.Run(t, "dexpire", "-a", "-h0", "-v")
	b, _ := os.ReadFile(filepath.Join(s.Dir, "log", "general.log"))
	// both spools were looked at, not skipped
	for _, want := range []string{"Spool Object: 01", "Spool Object: 02"} {
		if !strings.Contains(out, want) {
			t.Errorf("dexpire -v does not mention %q:\n%s", want, out)
		}
	}
	if strings.Contains(string(b), "unable to statfs") || strings.Contains(out, "unable to statfs") {
		t.Errorf("dexpire could not statfs the spools:\n%s\n%s", b, out)
	}
}

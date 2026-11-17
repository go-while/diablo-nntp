package e2e

import (
	"os"
	"path/filepath"
	"testing"
)

// TestR2DexpireSpoolPathSlash (deep review r2 M12, D-239): dexpire keeps the history records
// of the spool directories it finds and marks every other record expired. It built the names
// of the directories from the spool's path as written, and the history side
// (ArticleFileName) added a slash of its own or compared the path with path_spool as written.
// With path_spool or a spool's "path" spelled with a trailing slash ("/news/spool/",
// "P.01/") no name matched, so dexpire -h1 marked every record of that spool expired and the
// feeder answered 430 for articles that were still stored.
//
// Spool paths are now kept without trailing slashes, and the spool home is compared without
// them. Nothing is old enough to expire here: after dexpire -a -h1 both articles must still
// be there (STAT 223). The last case (path_spool with a slash, an absolute spool path below
// it without one) worked before and must keep working.
func TestR2DexpireSpoolPathSlash(t *testing.T) {
	spool01 := func(path string) string {
		return "spool 01\n    minfree 1m\n    path " + path + "\nend\nmetaspool m\n    spool 01\nend\nexpire * m\n"
	}
	for _, tc := range []struct {
		name      string
		pathSpool string // path_spool; "" = the default
		spoolCtl  string // "" = DefaultSpoolCtl (spool 00)
		mkdir     string // spool partition to create, below the site home
	}{
		{"path_spool", "@home@/spool/news/", "", ""},
		{"relative", "", spool01("P.01/"), "spool/news/P.01"},
		{"below_home", "", spool01("@home@/spool/news/P.01/"), "spool/news/P.01"},
		{"elsewhere", "", spool01("@home@/other//"), "other"},
		{"path_spool_and_below_home", "@home@/spool/news/", spool01("@home@/spool/news/P.01"), "spool/news/P.01"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			o := SiteOptions{SpoolCtl: tc.spoolCtl}
			if tc.pathSpool != "" {
				o.Config = map[string]string{"path_spool": tc.pathSpool}
			}
			if tc.mkdir != "" {
				o.Setup = func(tb testing.TB, s *Site) {
					if err := os.MkdirAll(filepath.Join(s.Dir, tc.mkdir), 0o755); err != nil {
						tb.Fatal(err)
					}
				}
			}
			s := NewSite(t, o)
			ids := s.Inject(t, "e2e.test", 2)
			if tc.mkdir != "" {
				if d, _ := filepath.Glob(filepath.Join(s.Dir, tc.mkdir, "D.*")); len(d) == 0 {
					t.Fatalf("test setup: no spool directory in %s", tc.mkdir)
				}
			}
			p := s.Peer(t)
			for _, id := range ids {
				p.ExpectCmd(223, "STAT %s", id)
			}
			p.ExpectCmd(205, "QUIT")

			out := s.Run(t, "dexpire", "-a", "-h1", "-v")

			p = s.Peer(t)
			for _, id := range ids {
				if code, line := p.Cmd("STAT %s", id); code != 223 {
					t.Errorf("STAT %s after dexpire -a -h1: %q, want 223 (nothing was old enough to expire)", id, line)
				}
			}
			p.ExpectCmd(205, "QUIT")
			if t.Failed() {
				t.Logf("dexpire output:\n%s", out)
			}
		})
	}
}

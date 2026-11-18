package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestDexpireSpoolPathSlash (D-239): when dexpire updates the history, it keeps the records
// of the spool directories it finds and marks the other records expired. It built the names
// of the directories from the spool's path as written, and the history side
// (ArticleFileName) added a slash of its own or compared the path with path_spool as written.
// With path_spool or a spool's "path" spelled with a trailing slash ("/news/spool/",
// "P.01/") the names did not match in the slash cases below, so dexpire -h1 marked the history
// records of that spool expired and the feeder answered 430 for articles that were still
// stored.
//
// Spool paths are now kept without trailing slashes (a path / stays /), and the spool home is
// compared without them. Nothing is old enough to expire here: after dexpire -a -h1 both
// articles must still be there (STAT 223), and the feeder must still read them (ARTICLE 220
// with the article's body) through the paths it now builds. The last case (path_spool with a
// slash, an absolute spool path below it without one) worked before and must keep working.
func TestDexpireSpoolPathSlash(t *testing.T) {
	spool01 := func(path, more string) string {
		return "spool 01\n    minfree 1m\n    path " + path + "\n" + more + "end\nmetaspool m\n    spool 01\nend\nexpire * m\n"
	}
	for _, tc := range []struct {
		name      string
		pathSpool string // path_spool; "" = the default
		spoolCtl  string // "" = DefaultSpoolCtl (spool 00)
		mkdir     string // spool partition to create, below the site home
		glob      string // where the articles' directories must be, below the site home
	}{
		{"path_spool", "@home@/spool/news/", "", "", "spool/news/D.*"},
		{"path_spool_double", "@home@/spool//news//", "", "", "spool/news/D.*"},
		{"relative", "", spool01("P.01/", ""), "spool/news/P.01", "spool/news/P.01/D.*"},
		{"relative_spooldirs", "", spool01("P.01/", "    spooldirs 2\n"), "spool/news/P.01", "spool/news/P.01/N.0*/D.*"},
		{"below_home", "", spool01("@home@/spool/news/P.01/", ""), "spool/news/P.01", "spool/news/P.01/D.*"},
		{"elsewhere", "", spool01("@home@/other//", ""), "other", "other/D.*"},
		{"path_spool_and_below_home", "@home@/spool/news/", spool01("@home@/spool/news/P.01", ""), "spool/news/P.01", "spool/news/P.01/D.*"},
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
			if d, _ := filepath.Glob(filepath.Join(s.Dir, tc.glob)); len(d) == 0 {
				t.Fatalf("test setup: no spool directory %s", tc.glob)
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
			for _, id := range ids {
				code, line := p.Cmd("ARTICLE %s", id)
				if code != 220 {
					t.Errorf("ARTICLE %s after dexpire -a -h1: %q, want 220", id, line)
					continue
				}
				if body := strings.Join(p.ReadDotLines(), "\n"); !strings.Contains(body, "body of "+id) {
					t.Errorf("ARTICLE %s returned another article:\n%s", id, body)
				}
			}
			p.ExpectCmd(205, "QUIT")
			if t.Failed() {
				t.Logf("dexpire output:\n%s", out)
			}
		})
	}
}

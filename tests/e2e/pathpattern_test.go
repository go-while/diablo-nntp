package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestPathPatternPercent: a configured path pattern is expanded by substituting its first %s
// (the home or base directory) and %%, and every other character, including a stray %d or %n,
// is taken literally. Before, the pattern was the format of snprintf, so a % sequence in a
// path_* setting (or a dreader.access accessfile) read, or with %n wrote, memory that wasn't
// there. Checked through dpath, which expands the patterns as the daemons do.
func TestPathPatternPercent(t *testing.T) {
	s := NewSite(t, SiteOptions{
		NoStart: true,
		Config: map[string]string{
			"path_log": "%s/lo%dg%%x",
			"path_db":  "/var/tmp/e2e-db",
		},
	})
	for key, want := range map[string]string{
		"log":  s.Dir + "/lo%dg%x", // first %s is the home, %d literal, %% is %
		"home": s.Dir,
		"db":   "/var/tmp/e2e-db", // no %s at all: the pattern as it is
	} {
		got := strings.TrimSpace(s.Run(t, "dpath", key))
		if got != want {
			t.Errorf("dpath %s: %q, want %q", key, got, want)
		}
	}
}

// TestConfigPathPercent: the diablo.config path itself (-C, DIABLO_CONFIG_PATH) is expanded
// the same way, so a % in a directory name is taken literally. Before D-50's fix it was the
// format of xfopen(): a site in a directory with "%d" in its name couldn't be read ("Unable
// to open diablo.config using pattern ...").
func TestConfigPathPercent(t *testing.T) {
	s := NewSite(t, SiteOptions{NoStart: true})
	dir := filepath.Join(t.TempDir(), "c%dfg")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	conf := filepath.Join(dir, "diablo.config")
	if err := os.WriteFile(conf, []byte(s.ReadFile(t, "diablo.config")), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"-C", conf, "home"}, {"-C" + conf, "home"}} {
		out, err := s.TryRun("dpath", args...)
		if err != nil || strings.TrimSpace(out) != s.Dir {
			t.Errorf("dpath %s: %q (%v), want %q", strings.Join(args, " "), out, err, s.Dir)
		}
	}
}

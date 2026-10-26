package e2e

import (
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

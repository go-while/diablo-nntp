package e2e

import (
	"strings"
	"testing"
)

// TestConfigLongLine (D-82): LoadDiabloConfig() reads diablo.config with fgets() into a
// 256 byte buffer, so a comment line of 256 or more characters is read as two lines and
// the second half ("junk words ...", not a comment) makes every Diablo program exit with
// "Unknown diablo.config command".
func TestConfigLongLine(t *testing.T) {
	comment := "# " + strings.Repeat("a long comment, ", 26) + "tail words that are not a command"
	if len(comment) < 400 {
		t.Fatalf("test comment is only %d bytes", len(comment))
	}
	s := NewSite(t, SiteOptions{NoStart: true})
	// after the version line: commands before it are not looked at
	conf := strings.SplitN(s.ReadFile(t, "diablo.config"), "\n", 2)
	if !strings.HasPrefix(conf[0], "version") || len(conf) != 2 {
		t.Fatalf("unexpected diablo.config: %q", conf)
	}
	s.WriteFile(t, "diablo.config", conf[0]+"\n"+comment+"\n"+conf[1])

	// no server is running: dicmd gets past the configuration and then can't connect
	out, _ := s.TryRun("dicmd", "info")
	if strings.Contains(out, "Unknown diablo.config command") {
		t.Errorf("dicmd with a %d character comment line in diablo.config: %q", len(comment), out)
	}
}

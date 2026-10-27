package e2e

import (
	"strings"
	"testing"
)

func dgrpctlReaderSite(t *testing.T) *Site {
	return NewReaderSite(t, ReaderOptions{SpoolAddr: "127.0.0.1:1", Groups: []string{"e2e.test"}, NoStart: true})
}

func dgrpctlDesc(n int) string {
	return strings.TrimSpace(strings.Repeat("word of text ", n/13+1))[:n]
}

// dgrpctl newgroup copies its DESCRIPTION argument with strcpy into a 256-byte buffer.
func TestDgrpctlNewgroupLongDescription(t *testing.T) {
	s := dgrpctlReaderSite(t)
	out, err := s.TryRun("dgrpctl", "newgroup", "e2e.longdesc", "y", `""`, dgrpctlDesc(600))
	t.Logf("err=%v out=%q", err, out)
	if err != nil {
		t.Errorf("dgrpctl newgroup with a 600-character description failed: %v", err)
	}
}

// A description of 255 characters (fits dgrpctl's buffer) is stored percent-encoded, up to
// three bytes per character; groupinfo copies the encoded field into a 256-byte stack buffer.
func TestDgrpctlGroupinfoLongDescription(t *testing.T) {
	s := dgrpctlReaderSite(t)
	s.Run(t, "dgrpctl", "newgroup", "e2e.longdesc", "y", `""`, strings.Repeat("- ", 125))
	out, err := s.TryRun("dgrpctl", "groupinfo", "e2e.longdesc")
	gd := ""
	for _, l := range strings.Split(out, "\n") {
		if strings.HasPrefix(l, "  GD=") {
			gd = l
		}
	}
	t.Logf("err=%v GD line length %d", err, len(gd))
	if err != nil {
		t.Errorf("dgrpctl groupinfo failed on a group with a 250-character description: %v", err)
	}
}

package e2e

import (
	"strings"
	"testing"
)

// TestDicmdConfig: "dicmd config" applies a runtime setting and reports it, and rejects a
// path_ setting, which is read only at startup. Before 6-CUR-20261024-BWH it also reported
// "Set" for settings it had not applied.
func TestDicmdConfig(t *testing.T) {
	s := NewSite(t, SiteOptions{})

	out := s.Dicmd(t, "config", "maxconnect", "17")
	if !strings.Contains(out, "Set 'maxconnect' to '17'") {
		t.Errorf("dicmd config maxconnect 17: %q, want Set 'maxconnect' to '17'", out)
	}
	if out := s.Dicmd(t, "config", "maxconnect"); !strings.Contains(out, "17") {
		t.Errorf("dicmd config maxconnect after setting it: %q, want 17", out)
	}

	out = s.Dicmd(t, "config", "path_dumphist", "/bin/true")
	if strings.Contains(out, "Set '") || !strings.Contains(out, "Invalid") {
		t.Errorf("dicmd config path_dumphist /bin/true: %q, want it rejected as Invalid", out)
	}
}

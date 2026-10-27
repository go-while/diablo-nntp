package e2e

import (
	"strings"
	"testing"
)

// TestDexpireCtlLongComment (D-82): dexpire.ctl is read in 512-byte pieces. A comment line
// longer than that was split, and its tail read as a rule of its own; here the tail is
// "*:a16", a rule for every group placed before the real one. The whole comment must be
// skipped, so the e2e.* rule decides the size of the group's overview index.
func TestDexpireCtlLongComment(t *testing.T) {
	plain := overviewSize(t, "e2e.*:a4000\n")
	comment := "#" + strings.Repeat("c", 510) + "*:a16 is the tail of this comment\n"
	if got := overviewSize(t, comment+"e2e.*:a4000\n"); got != plain {
		t.Errorf("a %d character comment in dexpire.ctl changed the rules: overview file is %d bytes, %d without the comment", len(comment)-1, got, plain)
	}
}

// TestDexpireCtlLongPattern (D-82): a dexpire.ctl pattern is copied into oe_Wild[255]. A
// longer one ran over the fields behind it, and as the last rule over its list link
// (oe_Next), so a lookup that got past the rules before it followed a garbage pointer. Such
// a rule is now refused (logged and ignored); the rules before it still apply.
func TestDexpireCtlLongPattern(t *testing.T) {
	def := overviewSize(t, "other.*:a100\n")
	ctl := "other.*:a100\n" + strings.Repeat("x", 320) + ":a16\n"
	if got := overviewSize(t, ctl); got != def {
		t.Errorf("with a 320 character pattern as the last dexpire.ctl rule the overview file is %d bytes, want the default %d", got, def)
	}
}

package e2e

import (
	"strings"
	"testing"
)

// groupInfo returns the "key=value" fields that dgrpctl groupinfo prints for group.
func groupInfo(tb testing.TB, s *Site, group string) map[string]string {
	tb.Helper()
	out := s.Run(tb, "dgrpctl", "groupinfo", group)
	f := map[string]string{}
	for _, w := range strings.Fields(out) {
		if k, v, ok := strings.Cut(w, "="); ok {
			f[k] = v
		}
	}
	return f
}

// TestCheckgroupsExec: dgrpctl checkgroups exec on a list with a new group, a new moderated
// group, a changed description and an unchanged group, while one existing group of the
// hierarchy is missing from the list. New groups get their status (y, or m for
// "(Moderated)"), description and creation time; the changed group its description; the
// missing group is deleted. A second run changes nothing. Fails on main before D-36: new
// groups had only NB and NE, so groupinfo showed flag=n (no local posting) and no GD or CTS.
func TestCheckgroupsExec(t *testing.T) {
	s := NewReaderSite(t, ReaderOptions{
		SpoolAddr: "127.0.0.1:1",
		Groups:    []string{"e2e.test", "e2e.other", "e2e.gone"},
		NoStart:   true,
	})
	s.Run(t, "dgrpctl", "newgroup", "e2e.other", "y", `""`, "Other things.")
	s.WriteFile(t, "checkgroups", "e2e.test\tTest group, changed.\n"+
		"e2e.other\tOther things.\n"+
		"e2e.new\tA new group.\n"+
		"e2e.mod\tA moderated group. (Moderated)\n")
	cg := s.Dir + "/checkgroups"

	out := s.Run(t, "dgrpctl", "checkgroups", "exec", cg)
	for _, want := range []string{
		"Added group e2e.new", "Added group e2e.mod",
		"Modified group e2e.test", "Deleted group e2e.gone",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("checkgroups exec: no %q in\n%s", want, out)
		}
	}
	if strings.Contains(out, "e2e.other") {
		t.Errorf("checkgroups exec touched the unchanged e2e.other:\n%s", out)
	}

	for group, flag := range map[string]string{"e2e.new": "y", "e2e.mod": "m"} {
		f := groupInfo(t, s, group)
		if f["flag"] != flag {
			t.Errorf("%s: flag=%q, want %q", group, f["flag"], flag)
		}
		if f["GD"] == "" || f["GD"] == "?" {
			t.Errorf("%s: no description (GD=%q)", group, f["GD"])
		}
		if strings.HasPrefix(f["CTS"], "0=") || f["CTS"] == "" {
			t.Errorf("%s: no creation time (CTS=%q)", group, f["CTS"])
		}
	}
	if gd := groupInfo(t, s, "e2e.test")["GD"]; !strings.Contains(gd, "changed") {
		t.Errorf("e2e.test: description not updated, GD=%q", gd)
	}

	if out := s.Run(t, "dgrpctl", "checkgroups", "exec", cg); strings.Contains(out, ">> Checkgroups:") {
		t.Errorf("second run changed something:\n%s", out)
	}
}

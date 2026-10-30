package e2e

import (
	"strings"
	"testing"
)

// TestCheckgroupsLongLine (D-70): dgrpctl checkgroups read the list with fgets() into a
// 128-byte buffer, so a line of 128 bytes or more came in as two lines, and the tail was
// taken for another group. Here the tail of the first line starts with a valid group name
// ("e2e.phantom") exactly where the old buffer ended. The line is one group (e2e.long) with
// a long description; there is no group e2e.phantom, neither in the dry run nor after exec.
// A 600-byte description is cut to 255 bytes (as dgrpctl newgroup does) and a second run
// finds nothing to change.
func TestCheckgroupsLongLine(t *testing.T) {
	s := NewReaderSite(t, ReaderOptions{
		SpoolAddr: "127.0.0.1:1",
		Groups:    []string{"e2e.test"},
		NoStart:   true,
	})
	// "e2e.long\t" is 9 bytes; fgets(buf, 128) took 127 bytes of the line
	long := "e2e.long\t" + strings.Repeat("d", 118) + "e2e.phantom\ttail of a long description"
	// a description longer than the 255 bytes dgrpctl newgroup takes is cut there, not at 127
	huge := "e2e.huge\t" + strings.Repeat("h", 600)
	s.WriteFile(t, "checkgroups", "e2e.test\tThe test group.\n"+long+"\n"+huge+"\n")
	cg := s.Dir + "/checkgroups"

	out := s.Run(t, "dgrpctl", "checkgroups", cg)
	if !strings.Contains(out, "newgroup e2e.long y") {
		t.Errorf("dry run: no newgroup for e2e.long in\n%s", out)
	}
	if strings.Contains(out, "newgroup e2e.phantom") {
		t.Errorf("dry run: the tail of a long line became the group e2e.phantom:\n%s", out)
	}
	if !strings.Contains(out, "tail of a long description") {
		t.Errorf("dry run: the description of e2e.long is cut:\n%s", out)
	}

	out = s.Run(t, "dgrpctl", "checkgroups", "exec", cg)
	if strings.Contains(out, "e2e.phantom") {
		t.Errorf("exec: the tail of a long line became the group e2e.phantom:\n%s", out)
	}
	if gd := groupInfo(t, s, "e2e.long")["GD"]; !strings.Contains(gd, "tail") {
		t.Errorf("exec: e2e.long has a cut description, GD=%q", gd)
	}
	if out := s.Run(t, "dgrpctl", "groupinfo", "e2e.phantom"); strings.Contains(out, "flag=") {
		t.Errorf("exec: the group e2e.phantom exists:\n%s", out)
	}
	if gd := groupInfo(t, s, "e2e.huge")["GD"]; len(gd) != 255 {
		t.Errorf("exec: e2e.huge has a description of %d bytes, want 255", len(gd))
	}
	// a second run with the same list changes nothing
	if out := s.Run(t, "dgrpctl", "checkgroups", "exec", cg); strings.Contains(out, ">> Checkgroups:") {
		t.Errorf("second run changed something:\n%s", out)
	}
}

// TestCheckgroupsEmptyList (D-70): a list without any usable group (empty, or only lines
// that name no group) left the hierarchy pattern NULL, and the scan for groups to delete
// handed it to WildCmp(): SIGSEGV. The run must end normally and delete nothing.
func TestCheckgroupsEmptyList(t *testing.T) {
	for name, list := range map[string]string{
		"empty":   "",
		"noGroup": "\n# not a group line\nnodots\tA name without a dot.\n.bad..name\tx\n",
	} {
		list := list
		t.Run(name, func(t *testing.T) {
			s := NewReaderSite(t, ReaderOptions{
				SpoolAddr: "127.0.0.1:1",
				Groups:    []string{"e2e.test", "e2e.other"},
				NoStart:   true,
			})
			s.WriteFile(t, "checkgroups", list)
			cg := s.Dir + "/checkgroups"
			for _, args := range [][]string{{"checkgroups", cg}, {"checkgroups", "exec", cg}} {
				out, err := s.TryRun("dgrpctl", args...)
				if err != nil {
					t.Errorf("dgrpctl %s: %v\n%s", strings.Join(args, " "), err, out)
				}
				if strings.Contains(out, "rmgroup") || strings.Contains(out, "Deleted group") {
					t.Errorf("dgrpctl %s wants to delete groups:\n%s", strings.Join(args, " "), out)
				}
			}
			for _, g := range []string{"e2e.test", "e2e.other"} {
				if f := groupInfo(t, s, g); f["flag"] == "" {
					t.Errorf("group %s is gone after checkgroups with an unusable list", g)
				}
			}
		})
	}
}

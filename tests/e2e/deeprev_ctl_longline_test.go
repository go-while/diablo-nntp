package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestDeepRevCheckgroupsLongLines (L6-10): dcontrol.ctl is read three times for a
// checkgroups (dreaderd/control.c: ParseControl(), CgReadRules(), CgReadGrpRules()) and
// localgroups once (CgLocalGroups()). A line longer than the 255 characters of the buffer
// is skipped as a whole by ParseControl() and CgReadGrpRules(); CgReadRules() and
// CgLocalGroups() read with a plain fgets() and took the rest of such a line as a line of
// its own: a checkgroups rule, or a pattern of groups to protect, that nobody wrote.
func TestDeepRevCheckgroupsLongLines(t *testing.T) {
	// a comment of 255 characters with more text behind it on the same line
	long := "#" + strings.Repeat("x", 254)

	t.Run("dcontrol.ctl", func(t *testing.T) {
		s := cgSite(t, []string{"e2e.test"},
			"checkgroups:*:*:log=cg.log\n"+long+"checkgroups:*:*:doit\n",
			"e2e.test\tThe test group.",
			"e2e.new\tA new group.")
		logFile := filepath.Join(s.Dir, "log", "cg.log")
		var log string
		created := false
		limit := time.Now().Add(15 * time.Second)
		for time.Now().Before(limit) {
			b, _ := os.ReadFile(logFile)
			log = string(b)
			_, created = cgActive(t, s)["e2e.new"]
			if created || strings.Contains(log, "Checkgroups:") {
				break
			}
			time.Sleep(200 * time.Millisecond)
		}
		if created {
			t.Fatalf("e2e.new was created: the end of a comment line longer than the buffer was read as \"checkgroups:*:*:doit\"\ncg.log:\n%s", log)
		}
		if !strings.Contains(log, "Checkgroups: not acted on") {
			t.Errorf("cg.log does not say that the checkgroups was logged only (checkgroups:*:*:log):\n%s", log)
		}
	})

	t.Run("localgroups", func(t *testing.T) {
		s := cgSiteWith(t, []string{"e2e.test", "e2e.obsolete"},
			map[string]string{"dcontrol.ctl": "checkgroups:*:*:doit\n" + cgGroupLines, "localgroups": long + "e2e.obsolete\n"},
			nil,
			"e2e.test\tThe test group.")
		gone := false
		limit := time.Now().Add(15 * time.Second)
		for time.Now().Before(limit) && !gone {
			time.Sleep(200 * time.Millisecond)
			_, ok := cgActive(t, s)["e2e.obsolete"]
			gone = !ok
		}
		if !gone {
			t.Errorf("e2e.obsolete was not removed by the checkgroups: the end of a comment line of localgroups longer than the buffer was read as the pattern \"e2e.obsolete\"")
		}
	})

	// the log names the real limit: a line of 255 characters is read, one of 256 is not
	t.Run("localgroups limit", func(t *testing.T) {
		fits := "zz.fits." + strings.Repeat("x", 247)
		over := "zz.over." + strings.Repeat("x", 248)
		s := cgSiteWith(t, []string{"e2e.test", "e2e.obsolete"},
			map[string]string{"dcontrol.ctl": "checkgroups:*:*:doit\n" + cgGroupLines, "localgroups": fits + "\n" + over + "\n"},
			nil,
			"e2e.test\tThe test group.")
		want := "localgroups: line 2 too long (at most 255 characters), ignored"
		limit := time.Now().Add(15 * time.Second)
		for time.Now().Before(limit) && !strings.Contains(generalLog(s), want) {
			time.Sleep(200 * time.Millisecond)
		}
		log := generalLog(s)
		if !strings.Contains(log, want) {
			t.Errorf("general.log does not say %q for the localgroups line of 256 characters:\n%s", want, log)
		}
		if strings.Contains(log, "localgroups: line 1 too long") {
			t.Errorf("the localgroups line of 255 characters was refused as too long")
		}
	})
}

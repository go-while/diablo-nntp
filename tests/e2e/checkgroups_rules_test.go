package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Which dcontrol.ctl lines a checkgroups uses, as INN's controlchan: every checkgroups (and
// "all") line that matches the sender, from the last to the first; a line acts on the groups
// that match its pattern and no later line's; drop leaves groups out of the list; a verify line
// that does not verify is skipped; log and mail only report. See CgRun in dreaderd/control.c.

// cgLogLine is the first line of the dcontrol.ctl of these tests: it gives the control log
// cg.log, and as the earliest line it only reports what no other line took.
const cgLogLine = "all:*:*:log=cg.log\n"

// cgDone waits until dreaderd has processed the checkgroups, and returns the control log.
func cgDone(t *testing.T, s *Site) string {
	t.Helper()
	var log []byte
	waitFor(t, 10*time.Second, "the checkgroups in the control log", func() bool {
		log, _ = os.ReadFile(filepath.Join(s.Dir, "log", "cg.log"))
		return strings.Contains(string(log), ">> processing completed ok")
	})
	return string(log)
}

// cgWant checks which groups exist after the checkgroups.
func cgWant(t *testing.T, s *Site, log string, present, absent []string) {
	t.Helper()
	act := cgActive(t, s)
	for _, g := range present {
		if _, ok := act[g]; !ok {
			t.Errorf("%s is missing (LIST ACTIVE: %v)", g, act)
		}
	}
	for _, g := range absent {
		if _, ok := act[g]; ok {
			t.Errorf("%s is there (LIST ACTIVE: %v)", g, act)
		}
	}
	if t.Failed() {
		t.Logf("control log:\n%s", log)
	}
}

// TestCheckgroupsEveryLine: two doit lines for two hierarchies both act. Before, only the
// last matching line was used, so the groups of the first hierarchy were not checked.
func TestCheckgroupsEveryLine(t *testing.T) {
	s := cgSite(t, []string{"e2e.test", "e2e.old", "other.old"},
		cgLogLine+"checkgroups:*:e2e.*:doit\ncheckgroups:*:other.*:doit\n",
		"e2e.test\tThe test group.",
		"e2e.new\tNew in e2e.",
		"other.new\tNew in other.",
	)
	log := cgDone(t, s)
	cgWant(t, s, log, []string{"e2e.test", "e2e.new", "other.new"}, []string{"e2e.old", "other.old"})
}

// TestCheckgroupsLaterLineWins: a later log line for e2e.keep.* takes those groups from the
// earlier doit line for *: they are neither created nor removed, but reported in the log; the
// other groups of the message are checked by the doit line.
func TestCheckgroupsLaterLineWins(t *testing.T) {
	s := cgSite(t, []string{"e2e.test", "e2e.obsolete", "e2e.keep.old"},
		cgLogLine+"checkgroups:*:*:doit\ncheckgroups:*:e2e.keep.*:log\n",
		"e2e.test\tThe test group.",
		"e2e.keep.new\tOnly reported.",
	)
	log := cgDone(t, s)
	cgWant(t, s, log, []string{"e2e.test", "e2e.keep.old"}, []string{"e2e.obsolete", "e2e.keep.new"})
	if !strings.Contains(log, ">> Checkgroups: not acted on (newsgroups e2e.keep.*):\ne2e.keep.new\tOnly reported.\n") {
		t.Errorf("the log line did not report its group:\n%s", log)
	}
}

// TestCheckgroupsDrop: a later drop line leaves its groups out of the list, so the earlier doit
// line removes those that exist and does not create the others (INN's example: drop
// *binaries*).
func TestCheckgroupsDrop(t *testing.T) {
	s := cgSite(t, []string{"e2e.test", "e2e.binaries.old"},
		cgLogLine+"checkgroups:*:e2e.*:doit\ncheckgroups:*:*binaries*:drop\n",
		"e2e.test\tThe test group.",
		"e2e.binaries.old\tListed, but dropped.",
		"e2e.binaries.new\tListed, but dropped.",
		"e2e.new\tNew.",
	)
	log := cgDone(t, s)
	cgWant(t, s, log, []string{"e2e.test", "e2e.new"}, []string{"e2e.binaries.old", "e2e.binaries.new"})
}

// TestCheckgroupsAllDrop: when the last line drops every group, nothing is changed, even with
// an earlier doit line: no group is created and none removed.
func TestCheckgroupsAllDrop(t *testing.T) {
	for name, dcontrol := range map[string]string{
		"drop only":         cgLogLine + "checkgroups:*:*:drop\n",
		"drop after doit":   cgLogLine + "checkgroups:*:*:doit\ncheckgroups:*:*:drop\n",
		"drop for e2e only": cgLogLine + "checkgroups:*:e2e.*:doit\ncheckgroups:*:e2e.*:drop\n",
	} {
		t.Run(name, func(t *testing.T) {
			s := cgSite(t, []string{"e2e.test", "e2e.old"}, dcontrol,
				"e2e.test\tThe test group.",
				"e2e.new\tNew.",
			)
			log := cgDone(t, s)
			cgWant(t, s, log, []string{"e2e.test", "e2e.old"}, []string{"e2e.new"})
		})
	}
}

// TestCheckgroupsVerifySkipped: a verify line that the message does not pass (it has no
// signature) is skipped as if it were not there, so the earlier doit line acts on its groups.
func TestCheckgroupsVerifySkipped(t *testing.T) {
	s := cgSite(t, []string{"e2e.test", "e2e.old"},
		cgLogLine+"checkgroups:*:*:doit\ncheckgroups:*:e2e.*:verify-nobody@example.invalid\n",
		"e2e.test\tThe test group.",
		"e2e.new\tNew.",
	)
	log := cgDone(t, s)
	cgWant(t, s, log, []string{"e2e.test", "e2e.new"}, []string{"e2e.old"})
	if !strings.Contains(log, ">> Checkgroups: skipped (newsgroups e2e.*): pgp verification required") {
		t.Errorf("the skipped verify line is not in the log:\n%s", log)
	}
}

// TestCheckgroupsMaxdocheckgroups: INN's /maxdocheckgroups/ line sets the limit, and each change
// counts for the last such line that matches its group, not the first group of the message.
func TestCheckgroupsMaxdocheckgroups(t *testing.T) {
	eleven := []string{"e2e.test\tDescribed."}
	for _, n := range []string{"a", "b", "c", "d", "e", "f", "g", "h", "i", "j", "k"} {
		eleven = append(eleven, "e2e.new"+n+"\tNew group "+n+".")
	}
	t.Run("raised", func(t *testing.T) {
		s := cgSite(t, []string{"e2e.test"}, cgLogLine+"/maxdocheckgroups/:*:e2e.*:20\ncheckgroups:*:*:doit\n", eleven...)
		log := cgDone(t, s)
		cgWant(t, s, log, []string{"e2e.newa", "e2e.newk"}, nil)
	})
	t.Run("per group", func(t *testing.T) {
		// aaa.first sorts first; its limit (the default) is not the one of the e2e groups
		lines := append([]string{"aaa.first\tFirst."}, eleven...)
		s := cgSite(t, []string{"e2e.test"}, cgLogLine+"/maxdocheckgroups/:*:e2e.*:20\ncheckgroups:*:*:doit\n", lines...)
		log := cgDone(t, s)
		cgWant(t, s, log, []string{"aaa.first", "e2e.newa", "e2e.newk"}, nil)
	})
	t.Run("one limit exceeded", func(t *testing.T) {
		s := cgSite(t, []string{"e2e.test"}, cgLogLine+"/maxdocheckgroups/:*:e2e.newa|e2e.newb:1\ncheckgroups:*:*:doit\n", eleven[:4]...)
		log := cgDone(t, s)
		cgWant(t, s, log, nil, []string{"e2e.newa", "e2e.newb", "e2e.newc"})
		if !strings.Contains(log, "(limit 1 for e2e.newa|e2e.newb): NOT made") {
			t.Errorf("control log:\n%s", log)
		}
	})
}

package e2e

import (
	"strings"
	"testing"
)

// D-200: a checkgroups created and removed groups by its own checkgroups lines alone, so a site
// whose dcontrol.ctl drops every newgroup and rmgroup ("newgroup:*:*:drop") still got groups
// created and removed by any checkgroups that a checkgroups line acted on. A checkgroups now
// creates a group, or changes its moderated flag, only where a newgroup of it from the sender
// would be executed, and removes one only where an rmgroup would; what it does not make is
// logged as dgrpctl commands. Descriptions of groups that exist are updated as before.

// TestCheckgroupsFollowsNewgroupRmgroup: each case starts with e2e.test, e2e.obsolete and
// e2e.flip (not moderated) and gets a checkgroups (doit) that lists e2e.test, e2e.flip as
// moderated, e2e.new and e2e.no.new. Fails on main: every case creates e2e.new and e2e.no.new,
// removes e2e.obsolete and flags e2e.flip moderated.
func TestCheckgroupsFollowsNewgroupRmgroup(t *testing.T) {
	body := []string{
		"e2e.test\tThe test group, described again.",
		"e2e.flip\tNow moderated. (Moderated)",
		"e2e.new\tA new group.",
		"e2e.no.new\tA new group the newgroup lines keep out.",
	}
	for _, c := range []struct {
		name     string
		lines    string // after cgLogLine and the checkgroups line
		present  []string
		absent   []string
		flip     string // the flag of e2e.flip afterwards
		inLog    []string
		notInLog []string
		from     string   // the From: header, "" for the default
		extra    []string // headers after From:
	}{
		{"newgroup and rmgroup dropped", "newgroup:*:*:drop\nrmgroup:*:*:drop\n",
			[]string{"e2e.test", "e2e.obsolete", "e2e.flip"}, []string{"e2e.new", "e2e.no.new"}, "y",
			[]string{"2 to add, 1 to change, 1 to remove: not allowed by the newgroup/rmgroup lines",
				"dgrpctl newgroup e2e.new y", "dgrpctl newgroup e2e.no.new y", "dgrpctl modgroup e2e.flip m", "dgrpctl rmgroup e2e.obsolete"},
			[]string{"Added group", "Deleted group", "Changed group"}, "", nil},
		{"no newgroup or rmgroup line", "",
			[]string{"e2e.test", "e2e.obsolete", "e2e.flip"}, []string{"e2e.new", "e2e.no.new"}, "y",
			[]string{"dgrpctl newgroup e2e.new y", "dgrpctl rmgroup e2e.obsolete"}, nil, "", nil},
		{"newgroup allowed, rmgroup dropped", "newgroup:*:*:doit\nrmgroup:*:*:drop\n",
			[]string{"e2e.test", "e2e.obsolete", "e2e.flip", "e2e.new", "e2e.no.new"}, nil, "m",
			[]string{"Added group e2e.new", "dgrpctl rmgroup e2e.obsolete"}, []string{"Deleted group"}, "", nil},
		{"newgroup for part of the hierarchy", "newgroup:*:e2e.*:doit\nnewgroup:*:e2e.no.*:log\nrmgroup:*:*:doit\n",
			[]string{"e2e.test", "e2e.flip", "e2e.new"}, []string{"e2e.obsolete", "e2e.no.new"}, "m",
			[]string{"Added group e2e.new", "Deleted group e2e.obsolete", "dgrpctl newgroup e2e.no.new y"}, nil, "", nil},
		{"break ends the newgroup lines", "newgroup:*:*:doit,break\nnewgroup:*:*:drop\nrmgroup:*:*:doit\n",
			[]string{"e2e.test", "e2e.flip", "e2e.new", "e2e.no.new"}, []string{"e2e.obsolete"}, "m",
			[]string{"Added group e2e.new"}, []string{"not allowed by the newgroup/rmgroup lines"}, "", nil},
		{"newgroup needs a signature the checkgroups line did not check", "newgroup:*:*:verify-e2e-key\nrmgroup:*:*:verify-e2e-key\n",
			[]string{"e2e.test", "e2e.obsolete", "e2e.flip"}, []string{"e2e.new", "e2e.no.new"}, "y",
			[]string{"dgrpctl newgroup e2e.new y", "dgrpctl rmgroup e2e.obsolete"}, nil, "", nil},
		// cgLogLine (all:*:*:log=cg.log) is the last line that matches a newgroup of e2e.no.new
		// and e2e.flip, as ParseControl() finds it: logged, not executed.
		{"an all line counts as for a newgroup", "newgroup:*:e2e.new:doit\nrmgroup:*:*:doit\n",
			[]string{"e2e.test", "e2e.new", "e2e.flip"}, []string{"e2e.obsolete", "e2e.no.new"}, "y",
			[]string{"Added group e2e.new", "dgrpctl newgroup e2e.no.new y", "dgrpctl modgroup e2e.flip m"}, nil, "", nil},
		// The newgroup/rmgroup lines are matched as for a real newgroup (CtlFromMatches(): the
		// From: header as it is, case-sensitive), not as the checkgroups lines are (Sender:,
		// reduced to a lower-case address). Each of these fails when they are matched the
		// checkgroups way.
		{"From: not Sender: picks the newgroup line",
			"newgroup:someone@example.net:*:drop\nrmgroup:someone@example.net:*:drop\n" +
				"newgroup:group-admin@example.org:*:doit\nrmgroup:group-admin@example.org:*:doit\n",
			[]string{"e2e.test", "e2e.flip", "e2e.new", "e2e.no.new"}, []string{"e2e.obsolete"}, "m",
			[]string{"Added group e2e.new", "Deleted group e2e.obsolete"}, []string{"not allowed by the newgroup/rmgroup lines"},
			"Group Admin <group-admin@example.org>", []string{"Sender: someone@example.net"}},
		{"the from pattern is case-sensitive",
			"newgroup:*:*:drop\nrmgroup:*:*:drop\nnewgroup:*E2E@EXAMPLE.INVALID*:*:doit\nrmgroup:*E2E@EXAMPLE.INVALID*:*:doit\n",
			[]string{"e2e.test", "e2e.obsolete", "e2e.flip"}, []string{"e2e.new", "e2e.no.new"}, "y",
			[]string{"dgrpctl newgroup e2e.new y", "dgrpctl rmgroup e2e.obsolete"}, []string{"Added group", "Deleted group"}, "", nil},
		{"the from pattern sees the display name",
			"newgroup:*:*:drop\nrmgroup:*:*:drop\nnewgroup:ISC Newsgroups *:*:doit\nrmgroup:ISC Newsgroups *:*:doit\n",
			[]string{"e2e.test", "e2e.flip", "e2e.new", "e2e.no.new"}, []string{"e2e.obsolete"}, "m",
			[]string{"Added group e2e.new", "Deleted group e2e.obsolete"}, []string{"not allowed by the newgroup/rmgroup lines"},
			"ISC Newsgroups <group-admin@example.org>", nil},
	} {
		t.Run(c.name, func(t *testing.T) {
			s := cgSiteArt(t, []string{"e2e.test", "e2e.obsolete", "e2e.flip"},
				map[string]string{"dcontrol.ctl": cgLogLine + "checkgroups:*:e2e.*:doit\n" + c.lines},
				cgArticle(t, c.from, c.extra, body...))
			log := cgDone(t, s)
			if !strings.Contains(log, ">> processing completed ok") {
				t.Errorf("the checkgroups failed:\n%s", log)
			}
			cgWant(t, s, log, c.present, c.absent)
			if f := cgActive(t, s)["e2e.flip"]; f != c.flip {
				t.Errorf("e2e.flip: flag %q, want %q\n%s", f, c.flip, log)
			}
			if d := iscDescriptions(iscList(t, s, "NEWSGROUPS"), "e2e.test")["e2e.test"]; d != "The test group, described again." {
				t.Errorf("e2e.test: description %q, the checkgroups updates it in every case", d)
			}
			for _, w := range c.inLog {
				if !strings.Contains(log, w) {
					t.Errorf("control log without %q:\n%s", w, log)
				}
			}
			for _, w := range c.notInLog {
				if strings.Contains(log, w) {
					t.Errorf("control log with %q:\n%s", w, log)
				}
			}
		})
	}
}

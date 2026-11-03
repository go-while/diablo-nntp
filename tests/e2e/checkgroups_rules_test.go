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

// cgDone waits until dreaderd has processed the checkgroups (ok or not), and returns the
// control log.
func cgDone(t *testing.T, s *Site) string {
	t.Helper()
	var log []byte
	waitFor(t, 30*time.Second, "the checkgroups in the control log", func() bool {
		log, _ = os.ReadFile(filepath.Join(s.Dir, "log", "cg.log"))
		return strings.Contains(string(log), ">> processing completed ok") || strings.Contains(string(log), ">> processing failed")
	})
	return string(log)
}

// cgSiteArt starts a reader site with the groups and files and feeds it art, a checkgroups
// article built by the test (cgArticle).
func cgSiteArt(t *testing.T, groups []string, files map[string]string, art []string) *Site {
	t.Helper()
	st, spool := spoolStore(t)
	s := NewReaderSite(t, ReaderOptions{Spool: spool, Groups: groups, Files: files})
	feedReader(t, s, st, "e2e.test", art)
	return s
}

// cgArticle is a checkgroups article with the body lines; from replaces the From: header and
// extra headers are added after it.
func cgArticle(t *testing.T, from string, extra []string, lines ...string) []string {
	t.Helper()
	a := newArticle(msgID(t), "e2e.test", "cmsg checkgroups", lines...)
	var out []string
	for _, l := range a {
		if strings.HasPrefix(l, "From: ") && from != "" {
			l = "From: " + from
		}
		out = append(out, l)
		if strings.HasPrefix(l, "From: ") {
			out = append(out, extra...)
			out = append(out, "Control: checkgroups")
		}
	}
	return out
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
	t.Run("maxcheckgroups keeps its meaning", func(t *testing.T) {
		// the first group of the message (aaa.first) picks the limit for all its changes
		lines := append([]string{"aaa.first\tFirst."}, eleven...)
		s := cgSite(t, []string{"e2e.test"}, cgLogLine+"maxcheckgroups:*:aaa.*:20\ncheckgroups:*:*:doit\n", lines...)
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

// TestCheckgroupsVerifyTakesGroups: a verify line that the real ISC checkgroups passes acts on
// its groups (sci.*) and takes them from the earlier doit line for *, which checks the rest.
func TestCheckgroupsVerifyTakesGroups(t *testing.T) {
	s, st, _ := iscSiteCtl(t, []string{"news.announce.newgroups"}, cgLogLine+
		"/maxdocheckgroups/:*:*:100000\ncheckgroups:*:*:doit\n"+
		"checkgroups:group-admin@isc.org:sci.*:verify-news.announce.newgroups\n")
	feedReader(t, s, st, "news.announce.newgroups", iscArticle(t, "checkgroups", "modern"))
	log := cgDone(t, s)
	cgWant(t, s, log, []string{"sci.math", "comp.lang.c"}, nil)
	for _, want := range []string{">> Checkgroups: verified (newsgroups sci.*)", "made (newsgroups sci.*)", "made (newsgroups *)"} {
		if !strings.Contains(log, want) {
			t.Errorf("control log lacks %q:\n%s", want, log)
		}
	}
	if strings.Count(log, " to add, ") != 2 {
		t.Errorf("want two checks (sci.* and the rest):\n%s", log)
	}
}

// TestCheckgroupsVerifyDgpBlocks: a line with verify-<key> and dgpverify can never verify
// (dgpverify is not implemented), so it leaves its groups alone, even after a later line with
// the same key passed. Before, the result of the later line was reused for the key and the
// earlier line created the groups.
func TestCheckgroupsVerifyDgpBlocks(t *testing.T) {
	s, st, _ := iscSiteCtl(t, []string{"news.announce.newgroups"}, cgLogLine+
		"/maxdocheckgroups/:*:*:100000\ncheckgroups:*:*:verify-news.announce.newgroups,dgpverify\n"+
		"checkgroups:*:sci.*:verify-news.announce.newgroups\n")
	feedReader(t, s, st, "news.announce.newgroups", iscArticle(t, "checkgroups", "modern"))
	log := cgDone(t, s)
	cgWant(t, s, log, []string{"sci.math"}, []string{"comp.lang.c"})
	if !strings.Contains(log, ">> Checkgroups: cannot verify, groups left alone (newsgroups *)") {
		t.Errorf("control log:\n%s", log)
	}
}

// TestCheckgroupsSenderAddress: the lines are matched against one address: Sender:, else the
// first address of From:, in lower case. A From: that lists another sender's address no longer
// matches that sender's lines.
func TestCheckgroupsSenderAddress(t *testing.T) {
	files := func() map[string]string {
		return map[string]string{"dcontrol.ctl": cgLogLine + "checkgroups:other@example.invalid:*:doit\n"}
	}
	body := []string{"e2e.test\tThe test group.", "e2e.new\tNew."}
	t.Run("second address in From", func(t *testing.T) {
		s := cgSiteArt(t, []string{"e2e.test"}, files(), cgArticle(t, "E2E <e2e@example.invalid>, other@example.invalid", nil, body...))
		log := cgDone(t, s)
		cgWant(t, s, log, []string{"e2e.test"}, []string{"e2e.new"})
	})
	t.Run("comment and first word", func(t *testing.T) {
		s := cgSiteArt(t, []string{"e2e.test"}, files(), cgArticle(t, "(other@example.invalid) e2e@example.invalid", nil, body...))
		log := cgDone(t, s)
		cgWant(t, s, log, []string{"e2e.test"}, []string{"e2e.new"})
	})
	t.Run("last angle address", func(t *testing.T) {
		s := cgSiteArt(t, []string{"e2e.test"}, files(), cgArticle(t, "E2E <e2e@example.invalid> Other <other@example.invalid>", nil, body...))
		log := cgDone(t, s)
		cgWant(t, s, log, []string{"e2e.test", "e2e.new"}, nil)
	})
	t.Run("two bare addresses", func(t *testing.T) {
		// become one word ("e2e@example.invalid__other@example.invalid"), as in INN
		s := cgSiteArt(t, []string{"e2e.test"}, files(), cgArticle(t, "e2e@example.invalid, other@example.invalid", nil, body...))
		log := cgDone(t, s)
		cgWant(t, s, log, []string{"e2e.test"}, []string{"e2e.new"})
	})
	t.Run("empty Sender, From is used", func(t *testing.T) {
		s := cgSiteArt(t, []string{"e2e.test"}, files(), cgArticle(t, "other@example.invalid", []string{"Sender: "}, body...))
		log := cgDone(t, s)
		cgWant(t, s, log, []string{"e2e.test", "e2e.new"}, nil)
	})
	t.Run("Sender wins, any case", func(t *testing.T) {
		s := cgSiteArt(t, []string{"e2e.test"}, files(), cgArticle(t, "", []string{"Sender: Other <OTHER@Example.Invalid>"}, body...))
		log := cgDone(t, s)
		cgWant(t, s, log, []string{"e2e.test", "e2e.new"}, nil)
	})
}

// TestCheckgroupsLineMatching: an unknown action makes the line ignored (it does not take
// groups from earlier lines); break ends the scan; a line for another sender is not used; a
// line's own =log file gets what it does.
func TestCheckgroupsLineMatching(t *testing.T) {
	body := []string{"e2e.test\tThe test group.", "e2e.new\tNew."}
	for name, dcontrol := range map[string]string{
		"unknown action": cgLogLine + "checkgroups:*:*:doit\ncheckgroups:*:e2e.*:doti\n",
		"break":          cgLogLine + "checkgroups:*:*:doit\ncheckgroups:*:zzz.*:log,break\ncheckgroups:*:*:drop\n",
		"other sender":   cgLogLine + "checkgroups:*:*:doit\ncheckgroups:nobody@example.invalid:*:drop\n",
	} {
		t.Run(name, func(t *testing.T) {
			s := cgSite(t, []string{"e2e.test"}, dcontrol, body...)
			log := cgDone(t, s)
			cgWant(t, s, log, []string{"e2e.test", "e2e.new"}, nil)
		})
	}
	t.Run("own log", func(t *testing.T) {
		// the last =log (cg.log) is the message's log; the e2e.* line has its own
		s := cgSite(t, []string{"e2e.test"}, "checkgroups:*:e2e.*:doit=e2e-cg.log\ncheckgroups:*:zzz.*:log=cg.log\n", body...)
		log := cgDone(t, s)
		cgWant(t, s, log, []string{"e2e.new"}, nil)
		own, _ := os.ReadFile(filepath.Join(s.Dir, "log", "e2e-cg.log"))
		if !strings.Contains(string(own), "made (newsgroups e2e.*)") || !strings.Contains(string(own), "<< Control: checkgroups") {
			t.Errorf("the line's own log:\n%s", own)
		}
		if strings.Contains(log, "made (newsgroups e2e.*)") {
			t.Errorf("the check went to cg.log too:\n%s", log)
		}
	})
}

// TestCheckgroupsPgpverifyStatus: a signer that pgpverify prints counts only with exit status 0.
// A fake path_home/dbin/pgpverify prints the key's name and exits 0 (the line acts) or 2 (the
// signature could not be checked: the line leaves its groups alone).
func TestCheckgroupsPgpverifyStatus(t *testing.T) {
	for _, tc := range []struct {
		status string
		made   bool
	}{{"0", true}, {"2", false}} {
		t.Run("exit "+tc.status, func(t *testing.T) {
			st, spool := spoolStore(t)
			s := NewReaderSite(t, ReaderOptions{Spool: spool, Groups: []string{"e2e.test"}, NoStart: true,
				Files: map[string]string{"dcontrol.ctl": cgLogLine + "checkgroups:*:*:verify-e2e-key\n"}})
			if err := os.MkdirAll(filepath.Join(s.Dir, "dbin"), 0o755); err != nil {
				t.Fatal(err)
			}
			s.WriteFile(t, "dbin/pgpverify", "#!/bin/sh\ncat > /dev/null\necho e2e-key\nexit "+tc.status+"\n")
			if err := os.Chmod(filepath.Join(s.Dir, "dbin", "pgpverify"), 0o755); err != nil {
				t.Fatal(err)
			}
			_, port, _ := strings.Cut(s.Addr, ":")
			startReader(t, s, "[127.0.0.1]:"+port)
			feedReader(t, s, st, "e2e.test", cgArticle(t, "", []string{"X-PGP-Sig: 1.0 Subject,Control,Message-ID e2e"},
				"e2e.test\tThe test group.", "e2e.new\tNew."))
			log := cgDone(t, s)
			if tc.made {
				cgWant(t, s, log, []string{"e2e.new"}, nil)
			} else {
				cgWant(t, s, log, nil, []string{"e2e.new"})
				if !strings.Contains(log, "cannot verify, groups left alone") {
					t.Errorf("control log:\n%s", log)
				}
			}
		})
	}
}

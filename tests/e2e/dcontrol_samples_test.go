package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// The dcontrol.ctl samples, next to the dbin directory of the build.
var dcontrolSamples = []string{"dcontrol.ctl", "reader/dcontrol.ctl", "feeder+reader/dcontrol.ctl"}

// dcontrolSample returns the text of a shipped sample, or skips the test.
func dcontrolSample(tb testing.TB, name string) string {
	tb.Helper()
	b, err := os.ReadFile(filepath.Join(bin(tb), "..", "samples", name))
	if err != nil {
		tb.Skipf("no sample %s: %v", name, err)
	}
	return string(b)
}

// TestDcontrolSamplesSyntax: every rule of the shipped dcontrol.ctl samples has the four fields
// and an action dreaderd knows (a mistake would silently make the rule match nothing or do
// nothing), cancel comes first, the log defaults stay ahead of the hierarchies, no catch-all
// rule for some senders ("all:") follows them (it would override the hierarchies' rules for
// those senders), and the three samples have the same rules.
func TestDcontrolSamplesSyntax(t *testing.T) {
	msgs := map[string]bool{"cancel": true, "all": true, "checkgroups": true, "newgroup": true,
		"rmgroup": true, "ihave": true, "sendme": true, "sendsys": true, "senduuname": true, "version": true}
	var rules [][]string
	for _, name := range dcontrolSamples {
		var got []string
		for n, l := range strings.Split(dcontrolSample(t, name), "\n") {
			if strings.TrimSpace(l) == "" || strings.HasPrefix(l, "#") {
				continue
			}
			f := strings.SplitN(l, ":", 4)
			if len(f) != 4 {
				t.Errorf("%s:%d: %q: not <message>:<from>:<newsgroups>:<action>", name, n+1, l)
				continue
			}
			if !msgs[f[0]] {
				t.Errorf("%s:%d: unknown message type %q", name, n+1, f[0])
			}
			for _, a := range strings.Split(f[3], ",") {
				a, _, _ = strings.Cut(a, "=")
				switch {
				case a == "doit", a == "drop", a == "log", a == "mail", a == "break",
					strings.HasPrefix(a, "verify-") && len(a) > 7:
				default:
					t.Errorf("%s:%d: unknown action %q", name, n+1, a)
				}
			}
			got = append(got, l)
		}
		if len(got) < 1000 || got[0] != "cancel:*:*:doit,break" {
			t.Errorf("%s: %d rules, first %q", name, len(got), got[0])
		}
		// the catch-all rules are the defaults at the top, before the first hierarchy
		seenGroup := false
		for _, l := range got {
			switch {
			case strings.HasPrefix(l, "newgroup:*:control|general"):
				seenGroup = true
			case seenGroup && strings.HasPrefix(l, "all:"):
				t.Errorf("%s: %q follows the hierarchy rules and overrides them for those senders", name, l)
			}
		}
		rules = append(rules, got)
	}
	for i := 1; i < len(rules); i++ {
		if strings.Join(rules[i], "\n") != strings.Join(rules[0], "\n") {
			t.Errorf("%s has other rules than %s", dcontrolSamples[i], dcontrolSamples[0])
		}
	}
}

// TestDcontrolSamplesPolicy: with each shipped sample, as dcontrol.ctl, a newgroup of the Big 8
// signed by ISC (real article, modern key) creates the group, an unsigned newgroup for a Big 8
// group does not, and an unsigned alt.* newgroup does, as in INN's control.ctl.
func TestDcontrolSamplesPolicy(t *testing.T) {
	for _, name := range dcontrolSamples {
		t.Run(name, func(t *testing.T) {
			ctl := dcontrolSample(t, name)
			s, st, _ := iscSiteCtl(t, []string{"news.announce.newgroups", "e2e.test"}, ctl)
			exists := func(g string) bool { return iscGroupNames(iscList(t, s, "ACTIVE"))[g] }
			forged := func(g string) []string {
				a := newArticle(msgID(t), "e2e.test", "cmsg newgroup "+g,
					"For your newsgroups file:", g+"\tA test group.")
				return append(append(a[:6:6], "Control: newgroup "+g), a[6:]...)
			}
			feedReader(t, s, st, "news.announce.newgroups", iscArticle(t, "newgroup", "modern"))
			feedReader(t, s, st, "e2e.test", forged("comp.e2e.forged"), forged("alt.e2e.open"))
			waitFor(t, 20*time.Second, "comp.lang.go created by the signed newgroup", func() bool { return exists("comp.lang.go") })
			waitFor(t, 20*time.Second, "alt.e2e.open created", func() bool { return exists("alt.e2e.open") })
			time.Sleep(iscRefusedWait)
			if exists("comp.e2e.forged") {
				t.Error("an unsigned newgroup created comp.e2e.forged")
			}
		})
	}
}

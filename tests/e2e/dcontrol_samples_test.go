package e2e

import (
	"fmt"
	testkit "github.com/go-while/nntp-testkit"
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
		cgFrom := map[string]int{} // <from> of a checkgroups rule -> line
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
			// the <newsgroups> of a checkgroups rule is not tested when the rule is chosen, so
			// such a rule for any sender would drop every checkgroups message after it
			if f[0] == "checkgroups" && f[2] != "*" && strings.Contains(f[1], "*") && !strings.Contains(f[1], "@") {
				t.Errorf("%s:%d: %q applies to the checkgroups of every sender", name, n+1, l)
			}
			// dreaderd uses the last checkgroups rule whose <from> matches; a second line for
			// the same sender (another hierarchy) would replace the first one
			if f[0] == "checkgroups" && f[1] != "*" {
				if prev, ok := cgFrom[f[1]]; ok {
					t.Errorf("%s:%d: a second checkgroups line for %s (line %d): join the hierarchies with |", name, n+1, f[1], prev)
				}
				cgFrom[f[1]] = n + 1
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
// group does not, and an unsigned alt.* newgroup does, as in INN's control.ctl; the ISC
// checkgroups updates the description of a group (no rule of a later hierarchy drops it).
func TestDcontrolSamplesPolicy(t *testing.T) {
	for _, name := range dcontrolSamples {
		t.Run(name, func(t *testing.T) {
			ctl := dcontrolSample(t, name)
			s, st, _ := iscSiteCtl(t, []string{"news.announce.newgroups", "e2e.test", "comp.ai"}, ctl)
			exists := func(g string) bool { return iscGroupNames(iscList(t, s, "ACTIVE"))[g] }
			forged := func(g string) []string {
				a := newArticle(msgID(t), "e2e.test", "cmsg newgroup "+g,
					"For your newsgroups file:", g+"\tA test group.")
				return append(append(a[:6:6], "Control: newgroup "+g), a[6:]...)
			}
			feedReader(t, s, st, "news.announce.newgroups", iscArticle(t, "newgroup", "modern"))
			feedReader(t, s, st, "e2e.test", forged("comp.e2e.forged"), forged("alt.e2e.open"))
			art := iscArticle(t, "checkgroups", "modern")
			want := iscExpected(t, art, "comp.ai")["comp.ai"]
			feedReader(t, s, st, "news.announce.newgroups", art)
			testkit.WaitFor(t, 20*time.Second, "the ISC description of comp.ai", func() bool {
				return iscDescriptions(iscList(t, s, "NEWSGROUPS"), "comp.ai")["comp.ai"] == want
			})
			testkit.WaitFor(t, 20*time.Second, "comp.lang.go created by the signed newgroup", func() bool { return exists("comp.lang.go") })
			testkit.WaitFor(t, 20*time.Second, "alt.e2e.open created", func() bool { return exists("alt.e2e.open") })
			time.Sleep(iscRefusedWait)
			if exists("comp.e2e.forged") {
				t.Error("an unsigned newgroup created comp.e2e.forged")
			}
		})
	}
}

// TestDcontrolSamplesCheckgroupsTwoHierarchies: a sender with checkgroups for two hierarchies
// (fletcher@cs.utexas.edu: austin.* and tx.*) is acted on for both. dreaderd uses the last rule whose
// <from> matches, so the sample has one line with both hierarchies; with a line each, the austin.*
// checkgroups changed nothing and still logged "processing completed ok".
func TestDcontrolSamplesCheckgroupsTwoHierarchies(t *testing.T) {
	for _, name := range dcontrolSamples {
		t.Run(name, func(t *testing.T) {
			ctl := dcontrolSample(t, name)
			st, spool := spoolStore(t)
			s := NewReaderSite(t, ReaderOptions{Spool: spool, Groups: []string{"austin.test", "tx.test", "e2e.test"},
				Files: map[string]string{"dcontrol.ctl": ctl}})
			checkgroups := func(hier, group string) []string {
				a := newArticle(msgID(t), "e2e.test", "cmsg checkgroups "+hier,
					group+"\tDescribed by the "+hier+" checkgroups.")
				for i, l := range a {
					if strings.HasPrefix(l, "From: ") {
						a[i] = "From: fletcher@cs.utexas.edu"
					}
				}
				return append(append(a[:6:6], "Control: checkgroups"), a[6:]...)
			}
			feedReader(t, s, st, "e2e.test", checkgroups("austin", "austin.test"), checkgroups("tx", "tx.test"))
			for _, g := range []string{"austin.test", "tx.test"} {
				want := "Described by the " + strings.SplitN(g, ".", 2)[0] + " checkgroups."
				testkit.WaitFor(t, 20*time.Second, "the checkgroups description of "+g, func() bool {
					return iscDescriptions(iscList(t, s, "NEWSGROUPS"), g)[g] == want
				})
			}
		})
	}
}

// dcontrolWild is WildOrCmp: lib/wildcmp.c's WildCmp ('*' and '?' only, case-sensitive) for each
// '|' alternative.
func dcontrolWild(w, s string) bool {
	for _, alt := range strings.Split(w, "|") {
		if dcontrolWildOne(alt, s) {
			return true
		}
	}
	return false
}

func dcontrolWildOne(w, s string) bool {
	if w == "" {
		return s == ""
	}
	switch w[0] {
	case '*':
		for i := 0; i <= len(s); i++ {
			if dcontrolWildOne(w[1:], s[i:]) {
				return true
			}
		}
		return false
	case '?':
		return s != "" && dcontrolWildOne(w[1:], s[1:])
	}
	return s != "" && s[0] == w[0] && dcontrolWildOne(w[1:], s[1:])
}

// dcontrolFromMatches is ParseControl's test of the From: line against the <from> of a rule.
func dcontrolFromMatches(pat, from string) bool {
	if dcontrolWild(pat, from) {
		return true
	}
	return len(pat) <= 128 && dcontrolWild(fmt.Sprintf("*<%s>*|* %s|%s *", pat, pat, pat), from)
}

// dcontrolWildMeet returns a string both WildCmp patterns match (no '|'), if there is one: a
// breadth-first walk over pairs of positions in the two patterns.
func dcontrolWildMeet(p, q string) (string, bool) {
	type state struct{ i, j int }
	type step struct {
		prev state
		c    byte // 0: a '*' matched nothing
	}
	seen := map[state]step{{0, 0}: {}}
	queue := []state{{0, 0}}
	add := func(from, to state, c byte) {
		if _, ok := seen[to]; !ok {
			seen[to] = step{from, c}
			queue = append(queue, to)
		}
	}
	for len(queue) > 0 {
		st := queue[0]
		queue = queue[1:]
		if st.i == len(p) && st.j == len(q) {
			var b []byte
			for cur := st; cur != (state{0, 0}); cur = seen[cur].prev {
				if c := seen[cur].c; c != 0 {
					b = append([]byte{c}, b...)
				}
			}
			return string(b), true
		}
		if st.i < len(p) && p[st.i] == '*' {
			add(st, state{st.i + 1, st.j}, 0)
		}
		if st.j < len(q) && q[st.j] == '*' {
			add(st, state{st.i, st.j + 1}, 0)
		}
		if st.i == len(p) || st.j == len(q) {
			continue
		}
		// one character both can take: a literal of either side, else any
		var cands []byte
		for _, c := range []byte{p[st.i], q[st.j]} {
			if c != '*' && c != '?' {
				cands = append(cands, c)
			}
		}
		if len(cands) == 0 {
			cands = []byte{'x'}
		}
		for _, c := range cands {
			ni, nj := st.i, st.j
			switch p[st.i] {
			case '*':
			case '?':
				ni++
			default:
				if p[st.i] != c {
					continue
				}
				ni++
			}
			switch q[st.j] {
			case '*':
			case '?':
				nj++
			default:
				if q[st.j] != c {
					continue
				}
				nj++
			}
			if ni != st.i || nj != st.j {
				add(st, state{ni, nj}, c)
			}
		}
	}
	return "", false
}

// TestDcontrolSamplesCheckgroupsSender: the Syntax test finds a second checkgroups line with the
// same <from>; this one also finds a sender matched by the <from> of another line, a wildcard
// (eric@cirr.com by eric@*cirr.com). The senders tried are the <from> of every checkgroups rule
// with each '*' and '?' read as "x", and for every two rules an address both their <from> match
// (a*@h and *b@h both match ab@h). For each, the last rule it matches with ParseControl's From
// test must cover the hierarchies of all the rules it matches with the same action, or its
// checkgroups for the others changes nothing; a hierarchy is covered when the last rule lists it
// or has a pattern that matches it (review.* covers review.narrow.*). Rules with another action
// (signed and unsigned) cannot be joined and are not compared.
func TestDcontrolSamplesCheckgroupsSender(t *testing.T) {
	type rule struct {
		line                 int
		from, groups, action string
	}
	for _, name := range dcontrolSamples {
		var rules []rule
		for n, l := range strings.Split(dcontrolSample(t, name), "\n") {
			f := strings.SplitN(l, ":", 4)
			if len(f) != 4 || f[0] != "checkgroups" || f[1] == "*" {
				continue
			}
			rules = append(rules, rule{n + 1, f[1], f[2], f[3]})
		}
		senders := map[string]bool{}
		for i, r := range rules {
			senders[strings.NewReplacer("*", "x", "?", "x").Replace(r.from)] = true
			for _, o := range rules[i+1:] {
				if o.action != r.action {
					continue
				}
				for _, a := range strings.Split(r.from, "|") {
					for _, b := range strings.Split(o.from, "|") {
						if w, ok := dcontrolWildMeet(a, b); ok {
							senders[w] = true
						}
					}
				}
			}
		}
		reported := map[string]bool{}
		for sender := range senders {
			from := "E2E <" + sender + ">"
			var match []rule
			for _, o := range rules {
				if dcontrolFromMatches(o.from, from) {
					match = append(match, o)
				}
			}
			if len(match) == 0 {
				t.Errorf("%s: no checkgroups rule matches the sender %s it was made from", name, from)
				continue
			}
			last := match[len(match)-1]
			for _, m := range match {
				if m.action != last.action {
					continue
				}
				for _, g := range strings.Split(m.groups, "|") {
					if key := fmt.Sprint(from, m.line, g); !dcontrolWild(last.groups, g) && !reported[key] {
						reported[key] = true
						t.Errorf("%s:%d: a checkgroups from %s for %s is limited to %s by line %d",
							name, m.line, from, g, last.groups, last.line)
					}
				}
			}
		}
	}
}

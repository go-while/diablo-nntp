package e2e

// How the rules of dnewsfeeds combine: a label with a peer feed table (feeds/<label>), grouprefs
// in the IFILTER, ISPAM and ESPAM labels, GLOBAL and grouprefs, the options set to -1
// (nortflush, domismatch), a reload that changes them, a GLOBAL label without "end" and a
// groupref to its own label (D-261, D-262, D-263, D-266, D-300, D-301).

import (
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/go-while/nntp-testkit/client"
)

// outLabel is an outgoing label: name with its own alias (so that no article comes back
// from it) and the rule lines rules.
func outLabel(name, rules string) string {
	return "label " + name + "\n    hostname 127.0.0.1\n    alias e2e." + name + "\n" + rules + "end\n"
}

// peerLabel lets PeerIP feed every group; its alias keeps the articles off its own queue.
const peerLabel = "label peer\n    inhost " + PeerIP + "\n    alias e2e.peer\n    addgroup *\nend\n"

// TestDnewsfeedsPeerTableRequiregroup (D-261): a label with a peer feed table (the file
// feeds/<label>, written by feedrset/feedadd/feedcommit and read after the label) keeps the
// requiregroup lines of dnewsfeeds: an article without the required group is held back, also
// when the table only adds groups. A requiregroup in the table counts beside the label's own
// ones (one of them is enough). Fails before the fix: the table emptied the label's
// requiregroup list, so e2e.other was fed, and a requiregroup in the table replaced the
// label's (e2e.need alone was held back).
func TestDnewsfeedsPeerTableRequiregroup(t *testing.T) {
	newsfeeds := GlobalLabel + outLabel("out", "    addgroup *\n    requiregroup e2e.need\n")
	out := []string{"out"}
	for _, c := range []struct {
		name  string
		table string // feeds/out; "" = no table
		cases map[string][]string
	}{
		{"no table", "", map[string][]string{
			"e2e.other": nil, "e2e.other,e2e.need": out, "e2e.need": out,
		}},
		{"table adds groups", "addgroup e2e.*\n", map[string][]string{
			"e2e.other": nil, "e2e.other,e2e.need": out, "e2e.need": out,
		}},
		{"empty table", "\n", map[string][]string{
			"e2e.other": nil, "e2e.need": out,
		}},
		{"table requires another group", "requiregroup e2e.alt\n", map[string][]string{
			"e2e.other": nil, "e2e.need": out, "e2e.alt": out, "e2e.other,e2e.alt": out,
		}},
	} {
		t.Run(c.name, func(t *testing.T) {
			s := NewSite(t, SiteOptions{Newsfeeds: newsfeeds, NoStart: true})
			if c.table != "" {
				s.WriteFile(t, "feeds/out", c.table)
			}
			for _, groups := range slices.Sorted(maps.Keys(c.cases)) {
				if got := matched(t, s, groups, 100, "x"); !slices.Equal(got, c.cases[groups]) {
					t.Errorf("feeds/out %q, Newsgroups %s: matched %v, want %v", c.table, groups, got, c.cases[groups])
				}
			}
		})
	}
}

// TestDnewsfeedsGrouprefIncomingFilter (D-262): a groupref in the IFILTER label takes effect:
// IFILTER refers to a groupdef that adds e2e.bad.*, so an article to e2e.bad.x is refused
// (437) and one to e2e.good is taken. Fails before the fix: the groupref was never resolved,
// IFILTER matched nothing and e2e.bad.x was taken (235).
func TestDnewsfeedsGrouprefIncomingFilter(t *testing.T) {
	s := NewSite(t, SiteOptions{
		Newsfeeds: DefaultNewsfeeds + "groupdef badgroups\n    addgroup e2e.bad.*\nend\n" +
			"label IFILTER\n    groupref badgroups\nend\n",
	})
	for _, c := range []struct{ groups, want string }{
		{"e2e.bad.x", "437"},
		{"e2e.good", "235"},
		{"e2e.good,e2e.bad.y", "437"},
	} {
		if line := offerGroups(t, s, c.groups); !strings.HasPrefix(line, c.want) {
			t.Errorf("IHAVE with Newsgroups: %s: %q, want %s", c.groups, line, c.want)
		}
	}
}

// feedSameBodyTo offers n articles to group with different Message-IDs and the same body over
// one connection and returns how many of them diablo stored.
func feedSameBodyTo(t *testing.T, s *Site, group string, n int, body string) int {
	t.Helper()
	p := s.Peer(t)
	defer p.ExpectCmd(205, "QUIT")
	var ids []string
	for i := 0; i < n; i++ {
		id := msgID(t)
		ids = append(ids, id)
		p.ExpectCmd(335, "IHAVE %s", id)
		p.MustSend(client.Stuff(newArticle(id, group, "same body", body)))
		if _, _, err := p.ReadStatus(); err != nil {
			t.Fatalf("IHAVE %s: %v", id, err)
		}
	}
	stored := 0
	for _, id := range ids {
		if code, _ := p.Cmd("STAT %s", id); code == 223 {
			stored++
		}
	}
	return stored
}

// spamcheckGroupdef is the groupdef the ISPAM and ESPAM labels below refer to.
const spamcheckGroupdef = "groupdef spamcheck\n    addgroup e2e.checked.*\nend\n"

// TestDnewsfeedsGrouprefSpamLabels (D-262): a groupref in the ISPAM and ESPAM labels takes
// effect. ISPAM (internalfilter B3, the same body offered 8 times) refers to a groupdef of
// e2e.checked.*: the copies to e2e.checked.x are filtered (fewer than 8 stored), those to
// e2e.unchecked are not (8 stored). ESPAM refers to the same groupdef: an article with the
// subject "spam" to e2e.checked.x goes to the external filter, which calls it spam, so it
// reaches the onlyspam feed and not the nospam one; the same article to e2e.unchecked does
// not go to the filter and reaches the nospam feed. Fails before the fix: the grouprefs were
// never resolved, so neither filter saw any article.
func TestDnewsfeedsGrouprefSpamLabels(t *testing.T) {
	t.Run("ISPAM", func(t *testing.T) {
		const n = 8
		s := NewSite(t, SiteOptions{
			Config:    map[string]string{"internalfilter": "B3"},
			Newsfeeds: DefaultNewsfeeds + spamcheckGroupdef + "label ISPAM\n    groupref spamcheck\nend\n",
		})
		if got := feedSameBodyTo(t, s, "e2e.unchecked", n, "identical body, not checked"); got != n {
			t.Errorf("e2e.unchecked (not in ISPAM): %d of %d articles with the same body stored, want all", got, n)
		}
		if got := feedSameBodyTo(t, s, "e2e.checked.x", n, "identical body, checked"); got >= n {
			t.Errorf("e2e.checked.x (ISPAM groupref spamcheck): all %d articles with the same body stored, want the internal filter to refuse some", n)
		}
	})

	t.Run("ESPAM", func(t *testing.T) {
		newsfeeds := strings.Replace(filterNewsfeeds, "label ESPAM\n    addgroup *\nend\n",
			spamcheckGroupdef+"label ESPAM\n    groupref spamcheck\nend\n", 1)
		if newsfeeds == filterNewsfeeds {
			t.Fatal("no ESPAM label in filterNewsfeeds")
		}
		s := NewSite(t, SiteOptions{
			Config:    map[string]string{"feederfilter": "@home@/filter"},
			Newsfeeds: newsfeeds,
			Setup: func(tb testing.TB, s *Site) {
				if err := os.WriteFile(filepath.Join(s.Dir, "filter"), []byte(s.expand(perDotFilter)), 0o755); err != nil {
					tb.Fatal(err)
				}
			},
		})
		arts := []struct {
			name, groups, subject string
			want                  string // the feed that gets it
		}{
			{"spam, checked", "e2e.checked.x", "spam", "spamonly"},
			{"ham, checked", "e2e.checked.x", "ham", OutLabel},
			{"spam, not checked", "e2e.unchecked", "spam", OutLabel},
		}
		ids := make([]string, len(arts))
		for i, a := range arts {
			ids[i] = msgID(t)
			injectLines(t, s, ids[i], newArticle(ids[i], a.groups, a.subject, "body"))
		}
		waitQueued(t, s, ids, func(i int) []string { return []string{arts[i].want} })
		for _, f := range []string{OutLabel, "spamonly"} {
			q := s.queue(t, f)
			for i, a := range arts {
				has := len(deepRevMissing(q, ids[i:i+1])) == 0
				if want := a.want == f; has != want {
					t.Errorf("%s: in dqueue/%s %v, want %v", a.name, f, has, want)
				}
			}
		}
	})
}

// TestDnewsfeedsGlobalNotReappliedInGroupref (D-263): GLOBAL is applied before the label's own
// lines and not again inside a groupref, so it does not override what the label set before
// that groupref. With GLOBAL "delgroup e2e.junk.*", a label "addgroup e2e.junk.wanted" then
// "groupref std" gets e2e.junk.wanted, as the same label without the groupref does; with
// GLOBAL "addgroup e2e.*", a label "delgroup e2e.private" then "groupref std" does not get
// e2e.private. A groupref in GLOBAL to a groupdef is applied to every label without recursing
// through GLOBAL: no "Infinite recursion" in the log (a "groupref GLOBAL" inside GLOBAL still
// refers to itself and is not tested here). Fails before the fix: GLOBAL, applied again at
// the start of std, held e2e.junk.wanted back and fed e2e.private, and a groupref in GLOBAL
// recursed through GLOBAL again until the recursion limit.
func TestDnewsfeedsGlobalNotReappliedInGroupref(t *testing.T) {
	const std = "groupdef std\n    addgroup e2e.comp.*\nend\n"
	for _, c := range []struct {
		name      string
		newsfeeds string
		cases     map[string][]string
	}{
		{"GLOBAL delgroup, label addgroup before a groupref",
			"label GLOBAL\n    delgroup e2e.junk.*\nend\n" + std +
				outLabel("ref", "    addgroup e2e.junk.wanted\n    groupref std\n") +
				outLabel("plain", "    addgroup e2e.junk.wanted\n"),
			map[string][]string{
				"e2e.junk.wanted": {"plain", "ref"},
				"e2e.junk.other":  nil,
				"e2e.comp.x":      {"ref"},
			}},
		{"GLOBAL addgroup, label delgroup before a groupref",
			"label GLOBAL\n    addgroup e2e.*\nend\n" + std +
				outLabel("ref", "    delgroup e2e.private\n    groupref std\n") +
				outLabel("plain", "    delgroup e2e.private\n"),
			map[string][]string{
				"e2e.private": nil,
				"e2e.public":  {"plain", "ref"},
				"e2e.comp.x":  {"plain", "ref"},
			}},
		{"groupref in GLOBAL",
			"label GLOBAL\n    groupref common\nend\n" + "groupdef common\n    addgroup e2e.common.*\nend\n" +
				outLabel("none", "") + outLabel("some", "    addgroup e2e.comp.*\n"),
			map[string][]string{
				"e2e.common.x": {"none", "some"},
				"e2e.comp.x":   {"some"},
				"e2e.other":    nil,
			}},
	} {
		t.Run(c.name, func(t *testing.T) {
			s := NewSite(t, SiteOptions{Newsfeeds: c.newsfeeds, NoStart: true})
			for _, groups := range slices.Sorted(maps.Keys(c.cases)) {
				if got := matched(t, s, groups, 100, "x"); !slices.Equal(got, c.cases[groups]) {
					t.Errorf("Newsgroups %s: matched %v, want %v", groups, got, c.cases[groups])
				}
			}
			if g := generalLog(s); strings.Contains(g, "Infinite recursion") {
				t.Errorf("dfeedtest logged a recursion:\n%s", g)
			}
		})
	}
}

// TestDnewsfeedsNortflushDomismatch (D-266): nortflush and domismatch, which set an option to
// -1, do what they say also where char is unsigned (arm64, ppc64le, s390x; here a build with
// -funsigned-char). GLOBAL has rtflush and nomismatch; the peer label has domismatch, the
// label "nort" nortflush, the label "rt" neither. dfeedtest -F shows -1 for both. An article
// whose first Path: element is not an alias of the peer gets ".MISMATCH!" in its stored
// Path:, one from the alias does not. The queue entry of "rt" is written at once, that of
// "nort" (decided before it) only by "dicmd flush". Fails before the fix on an unsigned char
// build: the options were 255, so the Path: was not checked and "nort" was flushed per line.
func TestDnewsfeedsNortflushDomismatch(t *testing.T) {
	// "nort" comes after "rt": the labels are kept in reverse, so the master writes the queue
	// entry of "nort" before that of "rt", and once "rt" has it, "nort" has had its turn.
	s := NewSite(t, SiteOptions{
		Newsfeeds: "label GLOBAL\n    rtflush\n    nomismatch\nend\n" +
			"label peer\n    inhost " + PeerIP + "\n    alias e2e.peer\n    addgroup *\n    domismatch\nend\n" +
			outLabel("rt", "    addgroup *\n") + outLabel("nort", "    addgroup *\n    nortflush\n"),
	})
	dump := s.Run(t, "dfeedtest", "-F")
	for label, want := range map[string][]string{
		"peer": {"PerLineFlushOpt: 1", "NoMisMatch     : -1"},
		"rt":   {"PerLineFlushOpt: 1", "NoMisMatch     : 1"},
		"nort": {"PerLineFlushOpt: -1", "NoMisMatch     : 1"},
	} {
		_, block, _ := strings.Cut(dump, "label "+label+"\n")
		block, _, _ = strings.Cut(block, "\nend\n")
		for _, w := range want {
			if !containsLine(strings.Split(block, "\n"), "  "+w) {
				t.Errorf("dfeedtest -F, label %s: no %q in\n%s", label, w, block)
			}
		}
	}

	foreign, own := msgID(t), msgID(t)
	a := newArticle(foreign, "e2e.test", "first Path element not an alias", "body")
	a[0] = "Path: e2e.elsewhere!not-for-mail"
	injectLines(t, s, foreign, a)
	injectLines(t, s, own, newArticle(own, "e2e.test", "first Path element an alias", "body"))

	p := s.Peer(t)
	for id, want := range map[string]bool{foreign: true, own: false} {
		p.ExpectCmd(221, "HEAD %s", id)
		path := headerValue(p.ReadDotLines(), "Path")
		if got := strings.Contains(path, ".MISMATCH!"); got != want {
			t.Errorf("stored Path: %q, MISMATCH %v, want %v (domismatch)", path, got, want)
		}
	}
	p.ExpectCmd(205, "QUIT")

	// read the queue files without "dicmd flush"
	raw := func(label string) string {
		b, _ := os.ReadFile(filepath.Join(s.Dir, "dqueue", label))
		return string(b)
	}
	deadline := time.Now().Add(15 * time.Second)
	for !strings.Contains(raw("rt"), own) {
		if time.Now().After(deadline) {
			t.Fatalf("dqueue/rt (rtflush from GLOBAL) has no %s after 15 s without a flush: %q", own, raw("rt"))
		}
		time.Sleep(100 * time.Millisecond)
	}
	for _, id := range []string{foreign, own} {
		if strings.Contains(raw("nort"), id) {
			t.Errorf("dqueue/nort has %s before a flush: nortflush did not keep it buffered", id)
		}
	}
	if missing := deepRevMissing(s.queue(t, "nort"), []string{foreign, own}); len(missing) != 0 {
		t.Errorf("dqueue/nort after dicmd flush: missing %v", missing)
	}
}

// TestDnewsfeedsRulesReload: a running diablo takes a changed dnewsfeeds with the rules above:
// a label removed (gone), a label added (added), GLOBAL changed, the groupdef a label refers
// to changed, and a requiregroup added to a label with a peer feed table (feeds/keep, which
// stays). Articles before the change go by the old rules, articles after the reload by the
// new ones. Takes up to a minute and a quarter: dnewsfeeds is checked once a minute, and a
// file younger than 10 s is left alone for that minute. Fails before the fixes of D-261 and
// D-263: before the change keep did not get e2e.junk.wanted (GLOBAL applied again inside
// std), after it keep got e2e.sci.x without e2e.need (the table dropped the requiregroup).
func TestDnewsfeedsRulesReload(t *testing.T) {
	keep := "    addgroup e2e.junk.wanted\n    groupref std\n"
	before := "label GLOBAL\n    delgroup e2e.junk.*\nend\n" + "groupdef std\n    addgroup e2e.comp.*\nend\n" +
		peerLabel + outLabel("keep", keep) + outLabel("gone", "    addgroup *\n")
	after := "label GLOBAL\n    addgroup e2e.global.*\nend\n" + "groupdef std\n    addgroup e2e.sci.*\nend\n" +
		peerLabel + outLabel("keep", keep+"    requiregroup e2e.need\n") + outLabel("added", "    addgroup *\n")
	s := NewSite(t, SiteOptions{
		Newsfeeds: before,
		Setup:     func(tb testing.TB, s *Site) { s.WriteFile(tb, "feeds/keep", "addgroup e2e.extra.*\n") },
	})
	type art struct {
		groups string
		want   []string // the feeds that get it
	}
	all := []string{"keep", "gone", "added"}
	send := func(arts []art) {
		t.Helper()
		ids := make([]string, len(arts))
		for i, a := range arts {
			ids[i] = msgID(t)
			injectLines(t, s, ids[i], newArticle(ids[i], a.groups, "reload "+a.groups, "body"))
		}
		waitQueued(t, s, ids, func(i int) []string { return arts[i].want })
		for _, f := range all {
			q := s.queue(t, f)
			for i, a := range arts {
				has := len(deepRevMissing(q, ids[i:i+1])) == 0
				if want := slices.Contains(a.want, f); has != want {
					t.Errorf("Newsgroups %s: in dqueue/%s %v, want %v", a.groups, f, has, want)
				}
			}
		}
	}

	send([]art{
		{"e2e.junk.wanted", []string{"keep", "gone"}},
		{"e2e.comp.x", []string{"keep", "gone"}},
		{"e2e.extra.y", []string{"keep", "gone"}},
		{"e2e.junk.other", []string{"gone"}},
		{"e2e.global.x,e2e.need", []string{"gone"}},
	})

	// diablo notices a change by the file's modification time in whole seconds
	time.Sleep(1100 * time.Millisecond)
	s.WriteFile(t, "dnewsfeeds", after)
	written := time.Now()
	// the master checks dnewsfeeds at its first pass in a new minute (it waits up to 5 s in
	// select()) and leaves a file younger than 10 s alone for that minute
	boundary := written.Truncate(time.Minute).Add(time.Minute)
	if boundary.Sub(written) < 11*time.Second {
		boundary = boundary.Add(time.Minute)
	}
	sleepUntilWall(t, boundary.Add(7*time.Second))

	send([]art{
		{"e2e.comp.x,e2e.need", []string{"added"}},
		{"e2e.sci.x", []string{"added"}},
		{"e2e.sci.x,e2e.need", []string{"keep", "added"}},
		{"e2e.extra.y,e2e.need", []string{"keep", "added"}},
		{"e2e.global.x,e2e.need", []string{"keep", "added"}},
	})
}

// TestDnewsfeedsLabelWithoutEnd (D-300): a label left open (no "end" before the next label,
// the next groupdef or the end of the file) is closed as "end" closes it. A GLOBAL left open
// stays GLOBAL: its "addgroup e2e.global.*" reaches every label (out gets e2e.global.x), it is
// no feed of its own (dfeedtest matches no "GLOBAL"), the labels find it ("grouplist GLOBAL
// does not exist" is not logged), and the missing "end" is logged. A label left open at the
// end of the file gets its peer feed table (feeds/out adds e2e.extra.*). Fails before the fix:
// GLOBAL left open was linked as a feed (Matched: GLOBAL for e2e.global.x) and every label
// logged that GLOBAL does not exist; at the end of the file nothing was logged and feeds/out
// was not read.
func TestDnewsfeedsLabelWithoutEnd(t *testing.T) {
	const openGlobal = "label GLOBAL\n    addgroup e2e.global.*\n"
	out := []string{"out"}
	withGlobal := map[string][]string{"e2e.global.x": out, "e2e.comp.x": out, "e2e.other": nil}
	for _, c := range []struct {
		name      string
		newsfeeds string
		table     string // feeds/out; "" = no table
		logged    string
		cases     map[string][]string
	}{
		{"GLOBAL before a label",
			openGlobal + outLabel("out", "    addgroup e2e.comp.*\n"), "",
			"no end before new label", withGlobal},
		{"GLOBAL before a groupdef",
			openGlobal + "groupdef std\n    addgroup e2e.comp.*\nend\n" + outLabel("out", "    groupref std\n"), "",
			"no end before new groupdef", withGlobal},
		{"GLOBAL at the end of the file",
			outLabel("out", "    addgroup e2e.comp.*\n") + openGlobal, "",
			"no end before the end of the file", withGlobal},
		{"label with a peer feed table at the end of the file",
			GlobalLabel + strings.TrimSuffix(outLabel("out", "    addgroup e2e.comp.*\n"), "end\n"), "addgroup e2e.extra.*\n",
			"no end before the end of the file",
			map[string][]string{"e2e.comp.x": out, "e2e.extra.x": out, "e2e.other": nil}},
	} {
		t.Run(c.name, func(t *testing.T) {
			s := NewSite(t, SiteOptions{Newsfeeds: c.newsfeeds, NoStart: true})
			if c.table != "" {
				s.WriteFile(t, "feeds/out", c.table)
			}
			for _, groups := range slices.Sorted(maps.Keys(c.cases)) {
				if got := matched(t, s, groups, 100, "x"); !slices.Equal(got, c.cases[groups]) {
					t.Errorf("Newsgroups %s: matched %v, want %v", groups, got, c.cases[groups])
				}
			}
			g := generalLog(s)
			if strings.Contains(g, "grouplist GLOBAL does not exist") {
				t.Errorf("the labels did not find GLOBAL:\n%s", g)
			}
			if !strings.Contains(g, c.logged) {
				t.Errorf("no %q in the log:\n%s", c.logged, g)
			}
		})
	}

	// the same on a running diablo: GLOBAL left open before the next label gets no queue file
	t.Run("running diablo", func(t *testing.T) {
		s := NewSite(t, SiteOptions{Newsfeeds: openGlobal + peerLabel + outLabel("out", "    addgroup e2e.comp.*\n")})
		id := msgID(t)
		injectLines(t, s, id, newArticle(id, "e2e.global.x", "GLOBAL left open", "body"))
		waitQueued(t, s, []string{id}, func(int) []string { return out })
		if q := s.queue(t, "GLOBAL"); len(q) != 0 {
			t.Errorf("dqueue/GLOBAL: %q, want no queue for GLOBAL", q)
		}
	})
}

// TestDnewsfeedsGrouprefSelf (D-301): a groupref to its own label ("groupref GLOBAL" inside
// GLOBAL, "groupref std" inside groupdef std) is ignored, logged once per line ("refers to
// itself, ignored"), and the rest of the label applies as before: out gets e2e.global.x from
// GLOBAL and e2e.comp.x from std. Fails before the fix: the groupref was followed into the
// same list down to the recursion limit, and "Infinite recursion in dnewsfeeds file!" was
// logged (the matches were the same).
func TestDnewsfeedsGrouprefSelf(t *testing.T) {
	out := []string{"out"}
	for _, c := range []struct {
		name      string
		newsfeeds string
		warning   string
		lines     int // self-references in the file
		cases     map[string][]string
	}{
		{"GLOBAL",
			"label GLOBAL\n    groupref GLOBAL\n    addgroup e2e.global.*\n    groupref GLOBAL\nend\n" +
				outLabel("out", "    addgroup e2e.comp.*\n"),
			"groupref GLOBAL in GLOBAL refers to itself, ignored", 2,
			map[string][]string{"e2e.global.x": out, "e2e.comp.x": out, "e2e.other": nil}},
		{"groupdef",
			GlobalLabel + "groupdef std\n    groupref std\n    addgroup e2e.comp.*\nend\n" + outLabel("out", "    groupref std\n"),
			"groupref std in std refers to itself, ignored", 1,
			map[string][]string{"e2e.comp.x": out, "e2e.other": nil}},
	} {
		t.Run(c.name, func(t *testing.T) {
			s := NewSite(t, SiteOptions{Newsfeeds: c.newsfeeds, NoStart: true})
			for _, groups := range slices.Sorted(maps.Keys(c.cases)) {
				if got := matched(t, s, groups, 100, "x"); !slices.Equal(got, c.cases[groups]) {
					t.Errorf("Newsgroups %s: matched %v, want %v", groups, got, c.cases[groups])
				}
			}
			g := generalLog(s)
			if strings.Contains(g, "Infinite recursion") {
				t.Errorf("the self-reference was followed:\n%s", g)
			}
			// dfeedtest loads dnewsfeeds once per run
			if n, want := strings.Count(g, c.warning), c.lines*len(c.cases); n != want {
				t.Errorf("%q logged %d times in %d loads, want %d (once per line):\n%s", c.warning, n, len(c.cases), want, g)
			}
		})
	}
}

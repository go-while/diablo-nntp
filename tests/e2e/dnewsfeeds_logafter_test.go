package e2e

import (
	"strings"
	"testing"
	"time"

	testkit "github.com/go-while/nntp-testkit"
	"github.com/go-while/nntp-testkit/fakeserver"
)

// dspooloutLinks runs dspoolout -n -v (print the dnewslink command lines, start nothing) and
// returns each label's dnewslink arguments, by the label of its "-b<label>.S%05d" (the
// verbose line writes each option and its value as one word).
func dspooloutLinks(t *testing.T, s *Site) map[string][]string {
	t.Helper()
	out, err := s.TryRun("dspoolout", "-n", "-v")
	noSanitizerReport(t, out)
	if err != nil {
		t.Fatalf("dspoolout -n -v: %v\n%.800s", err, out)
	}
	links := map[string][]string{}
	for _, line := range strings.Split(out, "\n") {
		f := strings.Fields(line)
		if len(f) == 0 || f[0] != "dnewslink" {
			continue
		}
		for _, a := range f[1:] {
			if b, ok := strings.CutPrefix(a, "-b"); ok {
				label, _, _ := strings.Cut(b, ".S")
				links[label] = f
			}
		}
	}
	return links
}

// logAfterArg is the -l argument of a dnewslink command line, or "" when there is none.
func logAfterArg(args []string) string {
	for _, a := range args {
		if strings.HasPrefix(a, "-l") {
			return a
		}
	}
	return ""
}

// TestDnewsfeedsLogAfter: the dnewsfeeds option "logafter N" sets the number of articles
// between the "mark" status lines dnewslink writes to the general log; dspoolout passes it
// as dnewslink -l N. 0 turns the periodic mark lines off. A label without it gets no -l, so
// dnewslink keeps its default of 1000, unless GLOBAL sets one; a label's own value wins.
// A value that is not a whole number is refused (logged) and not passed on. Before, dnewslink
// always logged every 1000 articles, which is a line every few seconds per link on a busy feed.
func TestDnewsfeedsLogAfter(t *testing.T) {
	label := func(name, extra string) string {
		return "label " + name + "\n    hostname 127.0.0.1\n    addgroup *\n" + extra + "end\n"
	}
	queue := "D.00000000/B.0000 <a@b> 0,1\n"

	t.Run("per label", func(t *testing.T) {
		feeds := GlobalLabel +
			label("zero", "    logafter 0\n") +
			label("big", "    logafter 50000\n") +
			label("none", "") +
			label("bad", "    logafter 12x\n")
		s := NewSite(t, SiteOptions{Newsfeeds: feeds, NoStart: true})
		for _, l := range []string{"zero", "big", "none", "bad"} {
			s.WriteFile(t, "dqueue/"+l, queue)
		}
		links := dspooloutLinks(t, s)
		for l, want := range map[string]string{"zero": "-l0", "big": "-l50000", "none": "", "bad": ""} {
			args, ok := links[l]
			if !ok {
				t.Errorf("no dnewslink line for label %s: %v", l, links)
				continue
			}
			if got := logAfterArg(args); got != want {
				t.Errorf("label %s: -l argument %q, want %q (%s)", l, got, want, strings.Join(args, " "))
			}
		}
	})

	t.Run("from GLOBAL", func(t *testing.T) {
		feeds := "label GLOBAL\n    logafter 7\nend\n" +
			label("plain", "") +
			label("own", "    logafter 3\n")
		s := NewSite(t, SiteOptions{Newsfeeds: feeds, NoStart: true})
		s.WriteFile(t, "dqueue/plain", queue)
		s.WriteFile(t, "dqueue/own", queue)
		links := dspooloutLinks(t, s)
		for l, want := range map[string]string{"plain": "-l7", "own": "-l3"} {
			if got := logAfterArg(links[l]); got != want {
				t.Errorf("label %s: -l argument %q, want %q (%s)", l, got, want, strings.Join(links[l], " "))
			}
		}
	})
}

// TestDnewslinkLogAfterMarks: dnewslink -l N writes a "mark" status line after every N offered
// articles, and -l 0 writes none (the final totals stay). dnewslink started its counter at -1,
// so -l 1 never logged a mark and the default ones came at 999, 1999, ... (go-while's review
// of #352); the counter now starts at 0. The totals restart with each pass (batch file or
// connlife session), and so does the mark count: with two batch files (-N 2) both get their
// marks. Before, the count kept growing while the totals restarted, so a later pass logged
// fewer marks or none.
func TestDnewslinkLogAfterMarks(t *testing.T) {
	for _, tc := range []struct {
		l       string
		minMark int
		maxMark int
	}{
		{"1", 3, 4}, // after each of the 3 articles, plus the closing mark of a run that logged marks
		{"0", 0, 0},
	} {
		t.Run("-l "+tc.l, func(t *testing.T) {
			peer, got := newPeer(t, &fakeserver.PeerOptions{})
			s := NewSite(t, SiteOptions{Newsfeeds: OutNewsfeeds,
				Config: map[string]string{"path_generallog": "%s/general.log"}})
			ids := s.Inject(t, "e2e.test", 3)
			s.waitQueue(t, OutLabel, 3)
			out := s.dnewslink(t, peer, OutLabel, "-l", tc.l)
			noSanitizerReport(t, out)
			if len(got.list()) != len(ids) {
				t.Fatalf("peer accepted %q, want %q\n%s", got.list(), ids, out)
			}
			var log string
			testkit.WaitFor(t, 10*time.Second, "dnewslink's final line in general.log", func() bool {
				log = generalLog(s)
				return strings.Contains(log, " final secs=")
			})
			if n := strings.Count(log, " mark secs="); n < tc.minMark || n > tc.maxMark {
				t.Errorf("dnewslink -l %s: %d mark lines, want %d..%d:\n%s", tc.l, n, tc.minMark, tc.maxMark, log)
			}
		})
	}
}

// TestDnewslinkLogAfterMarksTwoPasses: dnewslink -N 2 -l 1 over two batch files of 3 articles
// each logs a mark after every article of both passes.
func TestDnewslinkLogAfterMarksTwoPasses(t *testing.T) {
	peer, _ := newPeer(t, &fakeserver.PeerOptions{})
	s := NewSite(t, SiteOptions{Newsfeeds: OutNewsfeeds,
		Config: map[string]string{"path_generallog": "%s/general.log"}})
	s.Inject(t, "e2e.test", 3)
	s.waitQueue(t, OutLabel, 3)
	q := s.ReadFile(t, "dqueue/"+OutLabel)
	// the same three articles in two batch files: marks count offers, accepted or not
	s.WriteFile(t, "dqueue/"+OutLabel+".S00000", q)
	s.WriteFile(t, "dqueue/"+OutLabel+".S00001", q)
	out := s.dnewslink(t, peer, OutLabel+".S%05d", "-S", "0", "-N", "2", "-l", "1")
	noSanitizerReport(t, out)
	var log string
	testkit.WaitFor(t, 10*time.Second, "two final lines in general.log", func() bool {
		log = generalLog(s)
		return strings.Count(log, " final secs=") >= 2
	})
	if n := strings.Count(log, " mark secs="); n < 6 {
		t.Errorf("dnewslink -N 2 -l 1 over two batches of 3: %d mark lines, want at least 6:\n%s", n, log)
	}
}

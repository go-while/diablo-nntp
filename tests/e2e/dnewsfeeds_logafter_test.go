package e2e

import (
	"strings"
	"testing"
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

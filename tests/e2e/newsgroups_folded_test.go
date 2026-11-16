package e2e

import (
	"os"
	"strings"
	"testing"

	"github.com/go-while/nntp-testkit/client"
)

// foldedArticle returns an article whose Newsgroups: header is the given lines: the first is
// the value after "Newsgroups: ", each further one a continuation line (it begins with white
// space). extra lines go right after the Newsgroups: lines.
func foldedArticle(id string, ng []string, extra ...string) []string {
	art := newArticle(id, ng[0], "folded newsgroups", "body")
	for i, l := range art {
		if strings.HasPrefix(l, "Newsgroups: ") {
			rest := append(append(append([]string{}, ng[1:]...), extra...), art[i+1:]...)
			return append(art[:i+1], rest...)
		}
	}
	return art
}

// TestNewsgroupsFoldedActive: RFC 5536 3.1.4 allows folding white space around the commas of
// the Newsgroups: list, so the header may go on over continuation lines. The feeder read only
// the first line: with "activedrop yes" an article whose only group in the active file was on
// a continuation line was refused NotInActive, and a moderated group there let an unapproved
// article through (D-208). A continuation of another header is not part of the list.
func TestNewsgroupsFoldedActive(t *testing.T) {
	s := NewSite(t, SiteOptions{
		Config: map[string]string{
			"active":     "on",
			"activedrop": "yes",
			"xrefhost":   PathHost,
		},
		Setup: func(tb testing.TB, s *Site) {
			s.WriteFile(tb, "dactive.kp", "")
			s.Run(tb, "dgrpctl", "-s", "newgroup", ModGroup, "m")
			s.Run(tb, "dgrpctl", "-s", "newgroup", OpenGroup, "y")
		},
	})
	for _, c := range []struct {
		name  string
		ng    []string
		extra []string
		want  string
	}{
		{"comma, then fold", []string{"e2e.nogroup,", " " + OpenGroup}, nil, "235"},
		{"fold, then comma", []string{"e2e.nogroup", " ," + OpenGroup}, nil, "235"},
		{"tab fold, no comma", []string{"e2e.nogroup", "\t" + OpenGroup}, nil, "235"},
		{"three lines", []string{"e2e.nogroup,", " e2e.nogroup2,", "  " + OpenGroup}, nil, "235"},
		{"no active group on any line", []string{"e2e.nogroup,", " e2e.nogroup2"}, nil, "437"},
		{"moderated group on the fold", []string{OpenGroup + ",", " " + ModGroup}, nil, "437"},
		{"continuation of another header", []string{"e2e.nogroup"},
			[]string{"Summary: x", " " + OpenGroup}, "437"},
	} {
		id := msgID(t)
		p := s.Peer(t)
		p.ExpectCmd(335, "IHAVE %s", id)
		p.MustSend(client.Stuff(foldedArticle(id, c.ng, c.extra...)))
		line, err := p.ReadLine()
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		p.ExpectCmd(205, "QUIT")
		if !strings.HasPrefix(line, c.want) {
			t.Errorf("%s: Newsgroups: %q: %q, want %s", c.name, c.ng, line, c.want)
		}
	}
}

// TestNewsgroupsFoldedXref: the groups on the continuation lines of a folded Newsgroups:
// header get their Xref entries, as the groups on the first line do. Before, the article was
// stored with only the first line's groups in its Xref (D-208).
func TestNewsgroupsFoldedXref(t *testing.T) {
	s := deepRevXrefSite(t, nil)
	p := s.Peer(t)

	id := msgID(t)
	x := deepRevXref(t, p, id, foldedArticle(id, []string{"e2e.test,", " e2e.other,", "\te2e.third"}))
	if want := PathHost + " e2e.test:1 e2e.other:1 e2e.third:1"; x != want {
		t.Errorf("Xref of an article with a folded Newsgroups: header: %q, want %q", x, want)
	}
	p.ExpectCmd(205, "QUIT")
}

// TestNewsgroupsFoldedLongLine: a continuation line of Newsgroups: longer than the feeder's input
// buffer (one page with the default feederbuffersize) comes in pieces, and only the first piece is
// added to the list (D-209). The name at the end of that piece is cut, so it is left out: a cut
// name must not be taken for another group. Here e2e.test.missing, cut after "e2e.test", would
// make the article accepted into e2e.test. The padding walks the cut over the name at every
// offset around the page size.
func TestNewsgroupsFoldedLongLine(t *testing.T) {
	s := NewSite(t, SiteOptions{
		Config: map[string]string{
			"active":     "on",
			"activedrop": "yes",
			"xrefhost":   PathHost,
		},
		Setup: func(tb testing.TB, s *Site) {
			s.WriteFile(tb, "dactive.kp", "")
			s.Run(tb, "dgrpctl", "-s", "newgroup", OpenGroup, "y")
		},
	})
	p := s.Peer(t)
	ps := os.Getpagesize()
	for pad := ps - 512; pad <= ps+64; pad++ {
		id := msgID(t)
		p.ExpectCmd(335, "IHAVE %s", id)
		p.MustSend(client.Stuff(foldedArticle(id, []string{"e2e.nogroup,", strings.Repeat(" ", pad) + OpenGroup + ".missing"})))
		line, err := p.ReadLine()
		if err != nil {
			t.Fatalf("padding %d: %v", pad, err)
		}
		if !strings.HasPrefix(line, "437") {
			t.Errorf("padding %d: %q, want 437 (neither group is in the active file)", pad, line)
		}
	}
	p.ExpectCmd(205, "QUIT")
}

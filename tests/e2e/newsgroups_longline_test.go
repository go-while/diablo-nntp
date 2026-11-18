package e2e

import (
	"fmt"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/go-while/nntp-testkit/client"
)

// ngMaxValue is the most the feeder keeps of the Newsgroups: list: MAXLINE (lib/defs.h, 16384)
// less the terminating NUL. The value counts from the byte after "Newsgroups:", white space
// included, until the line has ended.
const ngMaxValue = 16384 - 1

// ngLongWindow returns the paddings to try for a Newsgroups: line whose value is fixed bytes plus
// the padding: the offsets just below the page size, where the feeder's first piece of a long line
// ends (feederbuffersize 1 is one page, less a small header, and bgets() starts a long line at the
// start of the buffer). The value is kept under ngMaxValue: with 16 KiB pages the window stops at
// the limit (the piece end is still below it); with larger pages the first piece ends past the
// limit, so no line under the limit comes in pieces, and only values that end just under the
// limit (one piece) are tried.
func ngLongWindow(t *testing.T, fixed int) (lo, hi int) {
	t.Helper()
	ps := os.Getpagesize()
	lo, hi = ps-200, ps-8
	if maxPad := ngMaxValue - fixed; hi > maxPad {
		hi = maxPad
	}
	if lo > hi-64 {
		t.Logf("page size %d: the first piece of a line ends past the %d-byte list limit; testing only values that end just under the limit", ps, ngMaxValue)
		lo = hi - 64
	}
	return lo, hi
}

// ngOffer offers the article on p and returns the first status line of the final IHAVE reply.
func ngOffer(t *testing.T, p *client.Conn, id string, art []string) string {
	t.Helper()
	p.ExpectCmd(335, "IHAVE %s", id)
	p.MustSend(client.Stuff(art))
	line, err := p.ReadLine()
	if err != nil {
		t.Fatalf("IHAVE %s: %v", id, err)
	}
	return line
}

// TestNewsgroupsLongLine: a Newsgroups: line longer than the feeder's input buffer comes in
// pieces. The feeder read only the first piece, less its last byte (D-209): a group after it was
// not seen (with "activedrop yes" an article whose only carried group came after the first piece
// was refused NotInActive), and a name cut at the end of the piece was looked up cut short
// (e2e.test.missing as e2e.test). The padding walks e2e.test and e2e.test.missing over the offsets
// where the first piece ends, with "wireformat" off and on (with it on, at one padding the piece
// ends with the CR of the line's CR LF). Each article with e2e.test is accepted, is numbered in
// e2e.test only, and is stored with its Newsgroups: line as it came; each article with
// e2e.test.missing is refused.
func TestNewsgroupsLongLine(t *testing.T) {
	for _, wire := range []string{"off", "on"} {
		t.Run("wireformat "+wire, func(t *testing.T) { ngLongLine(t, wire) })
	}
}

func ngLongLine(t *testing.T, wire string) {
	s := NewSite(t, SiteOptions{
		Config: map[string]string{
			"active":     "on",
			"activedrop": "yes",
			"xrefhost":   PathHost,
			"wireformat": wire,
		},
		Setup: func(tb testing.TB, s *Site) {
			s.WriteFile(tb, "dactive.kp", "")
			s.Run(tb, "dgrpctl", "-s", "newgroup", OpenGroup, "y")
		},
	})
	p := s.Peer(t)
	const first = "e2e.nogroup,"
	missing := OpenGroup + ".missing"
	lo, hi := ngLongWindow(t, len(" ")+len(first)+len(missing))
	bad, num := 0, 0
	fail := func(format string, args ...any) {
		if bad++; bad <= 5 {
			t.Errorf(format, args...)
		}
	}
	for pad := lo; pad <= hi; pad++ {
		ng := first + strings.Repeat(" ", pad) + OpenGroup
		id := msgID(t)
		if line := ngOffer(t, p, id, newArticle(id, ng, "long newsgroups", "body")); !strings.HasPrefix(line, "235") {
			fail("padding %d: Newsgroups: %s<%d spaces>%s: %q, want 235", pad, first, pad, OpenGroup, line)
		} else {
			num++
			p.ExpectCmd(221, "HEAD %s", id)
			head := p.ReadDotLines()
			if x, want := headerValue(head, "Xref"), fmt.Sprintf("%s %s:%d", PathHost, OpenGroup, num); x != want {
				fail("padding %d: Xref: %q, want %q", pad, x, want)
			}
			if !slices.Contains(head, "Newsgroups: "+ng) {
				fail("padding %d: the stored head does not hold the Newsgroups: line as it came", pad)
			}
		}

		ng = first + strings.Repeat(" ", pad) + missing
		id = msgID(t)
		if line := ngOffer(t, p, id, newArticle(id, ng, "long newsgroups", "body")); !strings.HasPrefix(line, "437") {
			fail("padding %d: Newsgroups: %s<%d spaces>%s: %q, want 437 (neither group is in the active file)", pad, first, pad, missing, line)
		}
	}
	if bad > 5 {
		t.Errorf("%d failures in all over %d paddings (%d..%d)", bad, hi-lo+1, lo, hi)
	}
	p.ExpectCmd(205, "QUIT")
}

// TestNewsgroupsLongLineAutoAdd: with "feederautoaddtoactive on" the feeder adds every group it
// sees to the active file. Before the fix it saw only the first piece of a long Newsgroups: line,
// so a group after it got no Xref entry, and a name cut at the end of the piece was added to the
// active file as a new group (e2e.ngl.second cut to e2e.ngl.sec, say). Every article gets exactly
// the two groups it names.
func TestNewsgroupsLongLineAutoAdd(t *testing.T) {
	s := deepRevXrefSite(t, map[string]string{"xrefhost": PathHost})
	p := s.Peer(t)
	const first, second = "e2e.ngl.first,", "e2e.ngl.second"
	lo, hi := ngLongWindow(t, len(" ")+len(first)+len(second))
	bad, num := 0, 0
	for pad := lo; pad <= hi; pad++ {
		id := msgID(t)
		line := ngOffer(t, p, id, newArticle(id, first+strings.Repeat(" ", pad)+second, "long newsgroups", "body"))
		if !strings.HasPrefix(line, "235") {
			if bad++; bad <= 5 {
				t.Errorf("padding %d: %q, want 235", pad, line)
			}
			continue
		}
		num++
		p.ExpectCmd(221, "HEAD %s", id)
		x := headerValue(p.ReadDotLines(), "Xref")
		if want := fmt.Sprintf("%s e2e.ngl.first:%d %s:%d", PathHost, num, second, num); x != want {
			if bad++; bad <= 5 {
				t.Errorf("padding %d: Xref: %q, want %q", pad, x, want)
			}
		}
	}
	if bad > 5 {
		t.Errorf("%d of %d paddings in all gave a wrong reply or Xref", bad, hi-lo+1)
	}
	p.ExpectCmd(205, "QUIT")
}

// TestNewsgroupsLongLineLimit: a Newsgroups: line longer than the feeder keeps of the list
// (ngMaxValue) is read up to that limit, also when the line comes in pieces: e2e.ngl.mid, under
// the limit, counts; the name the limit cuts is left out (e2e.ngl.after.missing, cut after
// e2e.ngl.after, would put the article into e2e.ngl.after); and nothing after it is read, neither
// the rest of the line (e2e.ngl.past) nor a continuation line (e2e.ngl.cont), so no later piece
// can be joined to a cut name. On 4 KiB pages e2e.ngl.mid comes after the first piece; on 8 KiB
// pages it is inside the first piece, and the old code fails there through e2e.ngl.cont (it added
// the continuation line). On 16 KiB pages the limit falls in the second piece; with pages over
// 16 KiB the line is one piece and the same limit applies to it. The next article is handled as
// usual.
func TestNewsgroupsLongLineLimit(t *testing.T) {
	groups := []string{OpenGroup, "e2e.ngl.mid", "e2e.ngl.after", "e2e.ngl.past", "e2e.ngl.cont"}
	s := NewSite(t, SiteOptions{
		Config: map[string]string{
			"active":     "on",
			"activedrop": "yes",
			"xrefhost":   PathHost,
		},
		Setup: func(tb testing.TB, s *Site) {
			s.WriteFile(tb, "dactive.kp", "")
			for _, g := range groups {
				s.Run(tb, "dgrpctl", "-s", "newgroup", g, "y")
			}
		},
	})
	p := s.Peer(t)

	// value: " e2e.test", padding, " e2e.ngl.mid" at about 8000, padding, then the cut name
	// starting 13 bytes before the limit, then ",e2e.ngl.past"
	const tag = "Newsgroups:"
	cut := "e2e.ngl.after.missing"
	start := ngMaxValue - len("e2e.ngl.after")
	head := " " + OpenGroup + strings.Repeat(" ", 8000) + " e2e.ngl.mid"
	line1 := tag + head + strings.Repeat(" ", start-len(head)-1) + " " + cut + ",e2e.ngl.past"
	if got := strings.Index(line1, cut) - len(tag); got != start {
		t.Fatalf("test layout: cut name at %d, want %d", got, start)
	}
	id := msgID(t)
	art := newArticle(id, "x", "long newsgroups", "body")
	for i, l := range art {
		if strings.HasPrefix(l, tag) {
			art = append(append(append([]string{}, art[:i]...), line1, " e2e.ngl.cont"), art[i+1:]...)
			break
		}
	}
	if line := ngOffer(t, p, id, art); !strings.HasPrefix(line, "235") {
		t.Fatalf("Newsgroups: line of %d bytes and a continuation: %q, want 235", len(line1), line)
	}
	p.ExpectCmd(221, "HEAD %s", id)
	if x, want := headerValue(p.ReadDotLines(), "Xref"), PathHost+" "+OpenGroup+":1 e2e.ngl.mid:1"; x != want {
		t.Errorf("Xref: %q, want %q (the names under the limit only; the cut name and all after it are left out)", x, want)
	}

	// the next article is handled as usual
	id = msgID(t)
	if line := ngOffer(t, p, id, newArticle(id, "e2e.ngl.after,e2e.ngl.cont", "next", "body")); !strings.HasPrefix(line, "235") {
		t.Fatalf("next article: %q, want 235", line)
	}
	p.ExpectCmd(221, "HEAD %s", id)
	if x, want := headerValue(p.ReadDotLines(), "Xref"), PathHost+" e2e.ngl.after:1 e2e.ngl.cont:1"; x != want {
		t.Errorf("Xref of the next article: %q, want %q", x, want)
	}
	p.ExpectCmd(205, "QUIT")
}

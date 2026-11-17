package e2e

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/go-while/nntp-testkit/client"
)

// Groups for the folded Xref tests: xfoldLocal and xfoldAfter are in dactive.kp, xfoldOther
// and xfoldOther2 are not.
const (
	xfoldLocal  = "e2e.xfold.local"
	xfoldAfter  = "e2e.xfold.after"
	xfoldOther  = "e2e.xfold.other"
	xfoldOther2 = "e2e.xfold.other2"
)

// xfoldMaxValue is the most the feeder keeps of a header value: MAXLINE (lib/defs.h, 16384)
// less the terminating NUL, the same limit as for Newsgroups:.
const xfoldMaxValue = 16384 - 1

// newXrefFoldSite starts diablo as an Xref slave that syncs NX ("active on", "feederxrefslave
// on", "feederxrefsync on") with "activedrop yes", and xfoldLocal and xfoldAfter in the active
// file.
func newXrefFoldSite(t *testing.T) *Site {
	t.Helper()
	return NewSite(t, SiteOptions{
		Config: map[string]string{
			"active":          "on",
			"activedrop":      "yes",
			"feederxrefslave": "on",
			"feederxrefsync":  "on",
		},
		Setup: func(tb testing.TB, s *Site) {
			s.WriteFile(tb, "dactive.kp", "")
			s.Run(tb, "dgrpctl", "-s", "newgroup", xfoldLocal, "y")
			s.Run(tb, "dgrpctl", "-s", "newgroup", xfoldAfter, "y")
		},
	})
}

// xfoldArticle returns an article for newsgroups whose last header lines are xref: the first is
// the whole "Xref: ..." line, each further one a continuation line or another header.
func xfoldArticle(id, newsgroups string, xref []string) []string {
	head, body := splitArticle(newArticle(id, newsgroups, "folded xref", "body"))
	return append(append(append(append([]string{}, head...), xref...), ""), body...)
}

// offerXfold offers the article on p and returns the first status line of the final IHAVE reply.
func offerXfold(t *testing.T, p *client.Conn, id string, art []string) string {
	t.Helper()
	p.ExpectCmd(335, "IHAVE %s", id)
	p.MustSend(client.Stuff(art))
	line, err := p.ReadLine()
	if err != nil {
		t.Fatalf("IHAVE %s: %v", id, err)
	}
	return line
}

// xfoldNX returns the NX field of group in the site's dactive.kp, or "" when the record has none.
func xfoldNX(t *testing.T, s *Site, group string) string {
	t.Helper()
	out := s.Run(t, "dkp", "-r", filepath.Join(s.Dir, "dactive.kp"), group)
	if !strings.HasPrefix(strings.TrimSpace(out), "key ") {
		t.Fatalf("%s not in dactive.kp: %q", group, out)
	}
	for _, kv := range strings.Fields(out) {
		if v, ok := strings.CutPrefix(kv, "NX="); ok {
			return v
		}
	}
	return ""
}

// xfoldAbsent fails the test unless group has no record in the site's dactive.kp.
func xfoldAbsent(t *testing.T, s *Site, group string) {
	t.Helper()
	out := s.Run(t, "dkp", "-r", filepath.Join(s.Dir, "dactive.kp"), group)
	if strings.HasPrefix(strings.TrimSpace(out), "key ") {
		t.Errorf("%s, not carried, has a record in dactive.kp: %q", group, out)
	}
}

// TestFeederXrefFoldedActiveDrop: an Xref slave that syncs NX reads the whole of a folded
// Xref: header (RFC 5536 allows folding, as for Newsgroups:, D-208). The feeder kept only the
// line that starts with "Xref:", so the groups on the continuation lines got no NX update; with
// "activedrop yes" an article whose Xref: named only groups we do not carry, all on a
// continuation line, was accepted, and one whose carried group was only on the continuation
// line was refused. A carried group on either line keeps the article; the stored article keeps
// the folded lines as they came; a continuation of another header after Xref: is not part of it.
func TestFeederXrefFoldedActiveDrop(t *testing.T) {
	s := newXrefFoldSite(t)
	p := s.Peer(t)

	// the only entry on the continuation line: accepted, NX set, the lines stored unchanged
	xref := []string{"Xref: master", " " + xfoldLocal + ":6"}
	id := msgID(t)
	if line := offerXfold(t, p, id, xfoldArticle(id, xfoldLocal, xref)); !strings.HasPrefix(line, "235") {
		t.Errorf("Xref: %q: %q, want 235", xref, line)
	}
	if nx := xfoldNX(t, s, xfoldLocal); nx != "0000000006" {
		t.Errorf("%s NX=%q after Xref: %q, want 0000000006", xfoldLocal, nx, xref)
	}
	p.ExpectCmd(221, "HEAD %s", id)
	head := p.ReadDotLines()
	if i := slices.Index(head, xref[0]); i < 0 || i+1 >= len(head) || head[i+1] != xref[1] {
		t.Errorf("stored head of %s does not hold the folded Xref: lines %q unchanged: %q", id, xref, head)
	}

	// three lines, a tab fold: every carried group is synced, the last entry for a group wins
	xref = []string{"Xref: master " + xfoldLocal + ":7", "\t" + xfoldAfter + ":8", "  " + xfoldLocal + ":9"}
	id = msgID(t)
	if line := offerXfold(t, p, id, xfoldArticle(id, xfoldLocal+","+xfoldAfter, xref)); !strings.HasPrefix(line, "235") {
		t.Errorf("Xref: %q: %q, want 235", xref, line)
	}
	if nx := xfoldNX(t, s, xfoldLocal); nx != "0000000009" {
		t.Errorf("%s NX=%q after Xref: %q, want 0000000009", xfoldLocal, nx, xref)
	}
	if nx := xfoldNX(t, s, xfoldAfter); nx != "0000000008" {
		t.Errorf("%s NX=%q after Xref: %q, want 0000000008", xfoldAfter, nx, xref)
	}

	// a carried and a not carried group, one on each line, either way round: kept, NX synced
	for _, c := range []struct {
		xref []string
		want string
	}{
		{[]string{"Xref: master " + xfoldLocal + ":13", " " + xfoldOther + ":14"}, "0000000013"},
		{[]string{"Xref: master " + xfoldOther + ":15", " " + xfoldLocal + ":16"}, "0000000016"},
	} {
		id := msgID(t)
		if line := offerXfold(t, p, id, xfoldArticle(id, xfoldLocal+","+xfoldOther, c.xref)); !strings.HasPrefix(line, "235") {
			t.Errorf("Xref: %q (one carried group): %q, want 235", c.xref, line)
		}
		if nx := xfoldNX(t, s, xfoldLocal); nx != c.want {
			t.Errorf("%s NX=%q after Xref: %q, want %s", xfoldLocal, nx, c.xref, c.want)
		}
	}

	// only groups we do not carry, on the first and the continuation line: still refused
	for _, xref := range [][]string{
		{"Xref: master", " " + xfoldOther + ":5"},
		{"Xref: master " + xfoldOther + ":5", " " + xfoldOther2 + ":6"},
	} {
		id := msgID(t)
		if line := offerXfold(t, p, id, xfoldArticle(id, xfoldOther+","+xfoldOther2, xref)); !strings.HasPrefix(line, "437") {
			t.Errorf("Xref: %q (no carried group): %q, want 437", xref, line)
		}
	}
	xfoldAbsent(t, s, xfoldOther)
	xfoldAbsent(t, s, xfoldOther2)

	// a continuation line of another header that follows Xref: is not an Xref: entry
	xref = []string{"Xref: master " + xfoldLocal + ":17", "Summary: x", " " + xfoldAfter + ":12"}
	id = msgID(t)
	if line := offerXfold(t, p, id, xfoldArticle(id, xfoldLocal, xref)); !strings.HasPrefix(line, "235") {
		t.Errorf("Xref: %q: %q, want 235", xref, line)
	}
	if nx := xfoldNX(t, s, xfoldLocal); nx != "0000000017" {
		t.Errorf("%s NX=%q after Xref: %q, want 0000000017", xfoldLocal, nx, xref)
	}
	if nx := xfoldNX(t, s, xfoldAfter); nx != "0000000008" {
		t.Errorf("%s NX=%q after a Summary: continuation %q, want 0000000008 (unchanged)", xfoldAfter, nx, xref)
	}
	p.ExpectCmd(205, "QUIT")
}

// TestFeederXrefLongLine: an Xref: line longer than the feeder's input buffer comes in pieces.
// The buffer is feederbuffersize (1 by default) rounded up to a page, less a small header, and
// bgets() starts a long line at the start of the buffer, so the first piece ends a little before
// the page size. The feeder kept only the first piece, less its last byte, so an entry after it
// got no NX update, and an entry cut at the end of the piece set NX to the cut number. The
// padding walks the entry over the offsets just below the page size (where the cut falls); each
// article carries its own number. The entry is kept under the 16383-byte value limit: with
// 16 KiB pages the sweep stops at the limit (the cut is still below it); with larger pages the
// first piece ends past the limit, so no line under the limit comes in pieces there, and the
// test only checks entries that end just under the limit (one piece).
func TestFeederXrefLongLine(t *testing.T) {
	s := newXrefFoldSite(t)
	p := s.Peer(t)
	ps := os.Getpagesize()
	const prefix = " master "                  // the value before the padding
	entryLen := len(xfoldLocal) + len(":") + 6 // 6 digits: 100000+pad stays below 1000000
	maxPad := xfoldMaxValue - len(prefix) - entryLen
	lo, hi := ps-200, ps-8
	if hi > maxPad {
		hi = maxPad
	}
	if lo > hi-64 {
		t.Logf("page size %d: the first piece of a line ends past the %d-byte value limit; testing only entries that end just under the limit", ps, xfoldMaxValue)
		lo = hi - 64
	}
	bad := 0
	for pad := lo; pad <= hi; pad++ {
		num := 100000 + pad
		xref := "Xref:" + prefix + strings.Repeat(" ", pad) + fmt.Sprintf("%s:%d", xfoldLocal, num)
		id := msgID(t)
		line := offerXfold(t, p, id, xfoldArticle(id, xfoldLocal, []string{xref}))
		nx := xfoldNX(t, s, xfoldLocal)
		if want := fmt.Sprintf("%010d", num); !strings.HasPrefix(line, "235") || nx != want {
			if bad++; bad <= 5 {
				t.Errorf("padding %d: %q, %s NX=%q, want 235 and NX=%s", pad, line, xfoldLocal, nx, want)
			}
		}
	}
	if bad > 5 {
		t.Errorf("%d of %d paddings in all gave a wrong reply or NX", bad, hi-lo+1)
	}
	p.ExpectCmd(205, "QUIT")
}

// xfoldFill returns entries for xfoldLocal (number 21), each with a space before it, as many as
// fit into room bytes, and their length.
func xfoldFill(room int) (string, int) {
	e := " " + xfoldLocal + ":21"
	k := room / len(e)
	return strings.Repeat(e, k), k * len(e)
}

// TestFeederXrefFoldedLimit: a folded Xref: longer than the feeder keeps of a header value
// (xfoldMaxValue, as for Newsgroups:) is read up to that limit: the entries before it are synced,
// the entry the limit cuts is left out (a cut "e2e.xfold.after:1234567" would set NX to 12), and
// the entries after it are not read. The article is accepted and the next article is handled as
// usual. Here the limit falls on a continuation line.
func TestFeederXrefFoldedLimit(t *testing.T) {
	s := newXrefFoldSite(t)
	p := s.Peer(t)

	// starting NX values
	xref := []string{"Xref: master " + xfoldLocal + ":3 " + xfoldAfter + ":4"}
	id := msgID(t)
	if line := offerXfold(t, p, id, xfoldArticle(id, xfoldLocal+","+xfoldAfter, xref)); !strings.HasPrefix(line, "235") {
		t.Fatalf("Xref: %q: %q, want 235", xref, line)
	}

	// The value the feeder builds: what follows "Xref:", then a space and each continuation
	// line. Fill it with entries for xfoldLocal, then put the cut entry where the limit falls 18
	// bytes into it ("e2e.xfold.after:12"), then entries past the limit.
	xref = []string{"Xref: master"}
	n := len(" master")
	filler, _ := xfoldFill(950)
	cut := xfoldAfter + ":1234567"
	start := xfoldMaxValue - 18 // index of the cut entry in the value
	for n+1+len(filler) < start-2 {
		xref = append(xref, filler)
		n += 1 + len(filler)
	}
	pad := start - n - 2 // the joining space and the line's own leading space come first
	xref = append(xref, " "+strings.Repeat(" ", pad)+cut)
	n += 2 + pad
	if n != start {
		t.Fatalf("test layout: cut entry at %d, want %d", n, start)
	}
	xref = append(xref, " "+xfoldLocal+":99 "+xfoldAfter+":98", " "+xfoldLocal+":97")
	id = msgID(t)
	if line := offerXfold(t, p, id, xfoldArticle(id, xfoldLocal+","+xfoldAfter, xref)); !strings.HasPrefix(line, "235") {
		t.Errorf("Xref: of %d lines over the limit: %q, want 235", len(xref), line)
	}
	if nx := xfoldNX(t, s, xfoldLocal); nx != "0000000021" {
		t.Errorf("%s NX=%q, want 0000000021 (the entries before the limit; those after it are not read)", xfoldLocal, nx)
	}
	if nx := xfoldNX(t, s, xfoldAfter); nx != "0000000004" {
		t.Errorf("%s NX=%q, want 0000000004 (the entry cut by the limit is left out)", xfoldAfter, nx)
	}

	// the next article is handled as usual
	xref = []string{"Xref: master " + xfoldAfter + ":6"}
	id = msgID(t)
	if line := offerXfold(t, p, id, xfoldArticle(id, xfoldAfter, xref)); !strings.HasPrefix(line, "235") {
		t.Errorf("Xref: %q after the long one: %q, want 235", xref, line)
	}
	if nx := xfoldNX(t, s, xfoldAfter); nx != "0000000006" {
		t.Errorf("%s NX=%q, want 0000000006", xfoldAfter, nx)
	}
	p.ExpectCmd(205, "QUIT")
}

// TestFeederXrefLongLineLimit: the limit falls inside one Xref: line longer than the input
// buffer, so the line comes in pieces and the piece that reaches the limit stops the reading:
// neither the rest of that line nor a continuation line after it is added. If reading went on,
// the value, trimmed back to the last whole entry, would take "e2e.xfold.after:97" from the
// continuation line. An entry after the first piece but under the limit is synced
// (e2e.xfold.after:5). With pages of 64 KiB or more the line is one piece and the same limit
// applies on the first copy.
func TestFeederXrefLongLineLimit(t *testing.T) {
	s := newXrefFoldSite(t)
	p := s.Peer(t)

	xref := []string{"Xref: master " + xfoldLocal + ":3 " + xfoldAfter + ":4"}
	id := msgID(t)
	if line := offerXfold(t, p, id, xfoldArticle(id, xfoldLocal+","+xfoldAfter, xref)); !strings.HasPrefix(line, "235") {
		t.Fatalf("Xref: %q: %q, want 235", xref, line)
	}

	// value: " master", entries, " e2e.xfold.after:5", padding, " e2e.xfold.after:1234567"
	// starting 18 bytes before the limit, more entries; then a continuation line
	const head = " master"
	mid := " " + xfoldAfter + ":5"
	cut := xfoldAfter + ":1234567"
	start := xfoldMaxValue - 18
	filler, fl := xfoldFill(start - 1 - len(head) - len(mid) - 64)
	pad := start - 1 - len(head) - fl - len(mid)
	line1 := "Xref:" + head + filler + mid + strings.Repeat(" ", pad) + " " + cut + " " + xfoldAfter + ":98 " + xfoldLocal + ":96"
	if got := strings.Index(line1, cut) - len("Xref:"); got != start {
		t.Fatalf("test layout: cut entry at %d, want %d", got, start)
	}
	xref = []string{line1, " " + xfoldAfter + ":97"}
	id = msgID(t)
	if line := offerXfold(t, p, id, xfoldArticle(id, xfoldLocal+","+xfoldAfter, xref)); !strings.HasPrefix(line, "235") {
		t.Errorf("Xref: line of %d bytes and a continuation: %q, want 235", len(line1), line)
	}
	if nx := xfoldNX(t, s, xfoldLocal); nx != "0000000021" {
		t.Errorf("%s NX=%q, want 0000000021 (the entries before the limit)", xfoldLocal, nx)
	}
	if nx := xfoldNX(t, s, xfoldAfter); nx != "0000000005" {
		t.Errorf("%s NX=%q, want 0000000005 (the last entry under the limit; the cut entry and those after it, also on the continuation line, are not read)", xfoldAfter, nx)
	}
	p.ExpectCmd(205, "QUIT")
}

package e2e

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-while/nntp-testkit/client"
)

// Groups for the feeder Xref slave + sync tests: xsyncLocal and xsyncAfter are in dactive.kp,
// xsyncOther and xsyncOther2 are not.
const (
	xsyncLocal  = "e2e.xsync.local"
	xsyncAfter  = "e2e.xsync.after"
	xsyncOther  = "e2e.xsync.other"
	xsyncOther2 = "e2e.xsync.other2"
)

// newXrefSyncSite starts diablo as an Xref slave that syncs NX ("active on", "feederxrefslave
// on", "feederxrefsync on"), with or without "activedrop yes", and xsyncLocal and xsyncAfter in
// the active file.
func newXrefSyncSite(t *testing.T, activedrop string) *Site {
	t.Helper()
	return NewSite(t, SiteOptions{
		Config: map[string]string{
			"active":          "on",
			"activedrop":      activedrop,
			"feederxrefslave": "on",
			"feederxrefsync":  "on",
		},
		Setup: func(tb testing.TB, s *Site) {
			s.WriteFile(tb, "dactive.kp", "")
			s.Run(tb, "dgrpctl", "-s", "newgroup", xsyncLocal, "y")
			s.Run(tb, "dgrpctl", "-s", "newgroup", xsyncAfter, "y")
		},
	})
}

// offerXref offers the site an article for newsgroups with the header "Xref: master <xref>" and
// returns the first status line of the final IHAVE reply.
func offerXref(t *testing.T, s *Site, newsgroups, xref string) string {
	t.Helper()
	id := msgID(t)
	art := newArticle(id, newsgroups, "xref "+xref, "body")
	head, body := splitArticle(art)
	art = append(append(append([]string{}, head...), "Xref: master "+xref, ""), body...)
	p := s.Peer(t)
	defer p.ExpectCmd(205, "QUIT")
	p.ExpectCmd(335, "IHAVE %s", id)
	p.MustSend(client.Stuff(art))
	line, err := p.ReadLine()
	if err != nil {
		t.Fatalf("IHAVE with Xref: master %s: %v", xref, err)
	}
	return line
}

// xsyncNX returns the NX field of group in the site's dactive.kp, or "" when the record has none.
func xsyncNX(t *testing.T, s *Site, group string) string {
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

// xsyncAbsent fails the test unless group has no record in the site's dactive.kp (the sync must
// not create one for a group we do not carry).
func xsyncAbsent(t *testing.T, s *Site, group string) {
	t.Helper()
	out := s.Run(t, "dkp", "-r", filepath.Join(s.Dir, "dactive.kp"), group)
	if strings.HasPrefix(strings.TrimSpace(out), "key ") {
		t.Errorf("%s, not carried, has a record in dactive.kp: %q", group, out)
	}
}

// TestFeederXrefSyncMissingGroupActiveDrop: an Xref slave that syncs NX, with "activedrop yes",
// keeps an article whose Xref: names a group it does not carry besides groups it carries, and
// sets the NX of each carried group, also of those listed after the missing one. The docs say
// activedrop drops an article only when none of its groups is in the active file; an article
// whose Xref: names only groups we do not carry is still dropped. Before the fix
// UpdateActiveNX() gave up at the first group not in dactive.kp: the article was refused
// NotInActive (recorded in history, so no other peer could deliver it here while that lasted),
// and the groups after it kept a stale NX.
func TestFeederXrefSyncMissingGroupActiveDrop(t *testing.T) {
	s := newXrefSyncSite(t, "yes")

	// the missing group first: kept, and the carried group after it is synced
	if line := offerXref(t, s, xsyncOther+","+xsyncLocal, xsyncOther+":5 "+xsyncLocal+":6"); !strings.HasPrefix(line, "235") {
		t.Errorf("Xref: master %s:5 %s:6: %q, want 235", xsyncOther, xsyncLocal, line)
	}
	if nx := xsyncNX(t, s, xsyncLocal); nx != "0000000006" {
		t.Errorf("%s NX=%q after Xref %s:6, want 0000000006", xsyncLocal, nx, xsyncLocal)
	}

	// the missing group between two carried ones: both carried groups are synced
	if line := offerXref(t, s, xsyncLocal+","+xsyncOther+","+xsyncAfter,
		xsyncLocal+":7 "+xsyncOther+":8 "+xsyncAfter+":9"); !strings.HasPrefix(line, "235") {
		t.Errorf("Xref with %s in the middle: %q, want 235", xsyncOther, line)
	}
	if nx := xsyncNX(t, s, xsyncLocal); nx != "0000000007" {
		t.Errorf("%s NX=%q, want 0000000007", xsyncLocal, nx)
	}
	if nx := xsyncNX(t, s, xsyncAfter); nx != "0000000009" {
		t.Errorf("%s NX=%q (listed after %s), want 0000000009", xsyncAfter, nx, xsyncOther)
	}

	// no carried group in Xref: still dropped, and no NX changes
	if line := offerXref(t, s, xsyncOther+","+xsyncOther2, xsyncOther+":10 "+xsyncOther2+":11"); !strings.HasPrefix(line, "437") {
		t.Errorf("Xref with no carried group: %q, want 437", line)
	}
	if nx := xsyncNX(t, s, xsyncLocal); nx != "0000000007" {
		t.Errorf("%s NX=%q after an article it is not in, want 0000000007", xsyncLocal, nx)
	}
	if nx := xsyncNX(t, s, xsyncAfter); nx != "0000000009" {
		t.Errorf("%s NX=%q after an article it is not in, want 0000000009", xsyncAfter, nx)
	}
	xsyncAbsent(t, s, xsyncOther)
	xsyncAbsent(t, s, xsyncOther2)
}

// TestFeederXrefSyncMissingGroupNoDrop: without activedrop the article is kept either way, but
// the NX of a carried group listed after a group we do not carry must still be synced, so that
// this host, once it becomes the Xref master, does not hand out numbers the old master already
// used. Before the fix that NX was not updated.
func TestFeederXrefSyncMissingGroupNoDrop(t *testing.T) {
	s := newXrefSyncSite(t, "no")
	for _, c := range []struct {
		groups, xref, want string
	}{
		{xsyncOther + "," + xsyncLocal, xsyncOther + ":5 " + xsyncLocal + ":6", "0000000006"}, // missing group first
		{xsyncLocal + "," + xsyncOther, xsyncLocal + ":8 " + xsyncOther + ":5", "0000000008"}, // missing group last
	} {
		if line := offerXref(t, s, c.groups, c.xref); !strings.HasPrefix(line, "235") {
			t.Errorf("Xref: master %s: %q, want 235", c.xref, line)
		}
		if nx := xsyncNX(t, s, xsyncLocal); nx != c.want {
			t.Errorf("%s NX=%q after Xref: master %s, want %s", xsyncLocal, nx, c.xref, c.want)
		}
	}
	xsyncAbsent(t, s, xsyncOther)
}

// TestFeederXrefSyncNumbers: a valid number is written to NX as the master sent it, with no
// forward-only rule: a lower number, a number above 32 bits and the largest 64-bit article number
// (9223372036854775807) are all written, and a group listed twice in one Xref: ends with the
// last number. The same number again is accepted and NX stays (the test cannot see whether it
// is written again). Each article is accepted.
func TestFeederXrefSyncNumbers(t *testing.T) {
	s := newXrefSyncSite(t, "yes")
	for _, c := range []struct {
		groups, xref, want string
	}{
		{xsyncLocal, xsyncLocal + ":6", "0000000006"},
		{xsyncLocal, xsyncLocal + ":6", "0000000006"},                            // the same number again
		{xsyncLocal, xsyncLocal + ":5", "0000000005"},                            // lower: written
		{xsyncLocal, xsyncLocal + ":4294967296", "4294967296"},                   // above 32 bits
		{xsyncLocal, xsyncLocal + ":9223372036854775807", "9223372036854775807"}, // LLONG_MAX
		{xsyncLocal, xsyncLocal + ":12 " + xsyncLocal + ":11", "0000000011"},     // listed twice: the last wins
	} {
		if line := offerXref(t, s, c.groups, c.xref); !strings.HasPrefix(line, "235") {
			t.Errorf("Xref: master %s: %q, want 235", c.xref, line)
		}
		if nx := xsyncNX(t, s, xsyncLocal); nx != c.want {
			t.Errorf("after Xref: master %s: %s NX=%q, want %s", c.xref, xsyncLocal, nx, c.want)
		}
	}
}

// TestFeederXrefSyncBadNumber: an Xref: entry for a carried group whose number is negative, not
// a number, or too large for a 64-bit article number leaves that group's NX as it is, and the
// article is accepted (also with "activedrop yes": the group is carried). Before the fix such a
// number was written: -5 was masked to about 2^63, 99999999999999999999 was clamped to the
// largest number, and "abc" set NX to 0, so a later switch to Xref master would start numbering
// from there.
func TestFeederXrefSyncBadNumber(t *testing.T) {
	s := newXrefSyncSite(t, "yes")
	if line := offerXref(t, s, xsyncLocal, xsyncLocal+":6"); !strings.HasPrefix(line, "235") {
		t.Fatalf("Xref: master %s:6: %q, want 235", xsyncLocal, line)
	}
	for _, num := range []string{"-5", "abc", "99999999999999999999", "9223372036854775808", "", "7x", "+8"} {
		xref := xsyncLocal + ":" + num
		if line := offerXref(t, s, xsyncLocal, xref); !strings.HasPrefix(line, "235") {
			t.Errorf("Xref: master %s: %q, want 235", xref, line)
		}
		if nx := xsyncNX(t, s, xsyncLocal); nx != "0000000006" {
			t.Errorf("after Xref: master %s: %s NX=%q, want 0000000006 (unchanged)", xref, xsyncLocal, nx)
		}
	}
	// a bad number for one group does not stop the sync of the next one
	if line := offerXref(t, s, xsyncLocal+","+xsyncAfter, xsyncLocal+":-5 "+xsyncAfter+":3"); !strings.HasPrefix(line, "235") {
		t.Errorf("Xref: master %s:-5 %s:3: %q, want 235", xsyncLocal, xsyncAfter, line)
	}
	if nx := xsyncNX(t, s, xsyncAfter); nx != "0000000003" {
		t.Errorf("%s NX=%q after %s:3, want 0000000003", xsyncAfter, nx, xsyncAfter)
	}
	if nx := xsyncNX(t, s, xsyncLocal); nx != "0000000006" {
		t.Errorf("%s NX=%q after %s:-5, want 0000000006 (unchanged)", xsyncLocal, nx, xsyncLocal)
	}
}

// TestFeederXrefSyncNoEntry: an Xref: line with only the host and no group:number entry is
// accepted with "activedrop yes", as before the fix (UpdateActiveNX() still returns 1 for it, not
// 0), and changes no NX.
func TestFeederXrefSyncNoEntry(t *testing.T) {
	s := newXrefSyncSite(t, "yes")
	id := msgID(t)
	art := newArticle(id, xsyncLocal, "xref host only", "body")
	head, body := splitArticle(art)
	art = append(append(append([]string{}, head...), "Xref: master", ""), body...)
	p := s.Peer(t)
	p.ExpectCmd(335, "IHAVE %s", id)
	p.MustSend(client.Stuff(art))
	p.ExpectStatus(235, "IHAVE with Xref: master and no group:number entry")
	p.ExpectCmd(205, "QUIT")
	if nx := xsyncNX(t, s, xsyncLocal); nx != "" {
		t.Errorf("%s NX=%q after an Xref with no entry, want none", xsyncLocal, nx)
	}
}

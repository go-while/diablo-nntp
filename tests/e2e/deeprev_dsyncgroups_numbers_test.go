package e2e

import (
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// deepRevSync runs dsyncgroups against a peer that lists the active lines, and returns the
// output.
func deepRevSync(t *testing.T, s *Site, active []string, args ...string) string {
	t.Helper()
	port := dsyncPeer{Active: active}.listen(t)
	a := append([]string{"-h", "127.0.0.1", "-p", strconv.Itoa(port)}, args...)
	return s.Run(t, "dsyncgroups", a...)
}

// deepRevNE returns the NE field of the group's record in the KP database db.
func deepRevNE(t *testing.T, db, group string) string {
	t.Helper()
	live, _, _ := kpRecords(t, db, group)
	if len(live) != 1 {
		t.Fatalf("%s: %d live records for %s: %q", db, len(live), group, live)
	}
	for _, f := range strings.Fields(live[0]) {
		if v, ok := strings.CutPrefix(f, "NE="); ok {
			return v
		}
	}
	t.Fatalf("no NE in %q", live[0])
	return ""
}

// TestDeepRevDsyncgroupsLargeNumbers (L7-07): article numbers are 64 bit, but dsyncgroups
// kept the distance between the local and the remote end number in an int and computed the
// percentage for -P as int * 100.
//
//   - a remote end number 5000000000 against a local 10 gave NE=0705032704 (the distance cut
//     to 32 bit) instead of 5000000000;
//   - with -P 50, a local 1000 and a remote 30001000, the percentage (3000000 %) overflowed
//     to a negative number, so the limit of 50 % was not applied and NE became 30001000
//     instead of 1500.
func TestDeepRevDsyncgroupsLargeNumbers(t *testing.T) {
	t.Run("distance", func(t *testing.T) {
		s := NewSite(t, SiteOptions{NoStart: true})
		db := filepath.Join(s.Dir, "deeprev.kp")
		deepRevSync(t, s, []string{"alpha 10 1 y"}, "-F", db, "-g", "-a", "-NE")
		if ne := deepRevNE(t, db, "alpha"); ne != "0000000010" {
			t.Fatalf("setup: NE=%s, want 0000000010", ne)
		}
		out := deepRevSync(t, s, []string{"alpha 5000000000 1 y"}, "-f", db, "-a", "-NE", "-v")
		if ne := deepRevNE(t, db, "alpha"); ne != "5000000000" {
			t.Errorf("NE=%s after a sync with a remote end number 5000000000\n%s", ne, out)
		}
	})
	t.Run("percent", func(t *testing.T) {
		s := NewSite(t, SiteOptions{NoStart: true})
		db := filepath.Join(s.Dir, "deeprev.kp")
		deepRevSync(t, s, []string{"alpha 1000 1 y"}, "-F", db, "-g", "-a", "-NE")
		if ne := deepRevNE(t, db, "alpha"); ne != "0000001000" {
			t.Fatalf("setup: NE=%s, want 0000001000", ne)
		}
		out := deepRevSync(t, s, []string{"alpha 30001000 1 y"}, "-f", db, "-a", "-NE", "-P", "50", "-v")
		if ne := deepRevNE(t, db, "alpha"); ne != "0000001500" {
			t.Errorf("NE=%s after a sync limited to 50%% (-P 50) from 1000, want 0000001500\n%s", ne, out)
		}
	})
}

// TestDeepRevDsyncgroupsNumberBounds (Terra, second review of L7-07): dsyncgroups formatted
// the NB, NE and NX numbers with sprintf("%010lld") into 16-byte buffers. A number of 16
// digits or more from the remote's LIST ACTIVE ran past the buffer on the stack. Numbers above
// 10^14 (far above any real group) are now refused with a message, and the group keeps its
// numbers; a number just below the bound is still taken.
func TestDeepRevDsyncgroupsNumberBounds(t *testing.T) {
	s := NewSite(t, SiteOptions{NoStart: true})
	db := filepath.Join(s.Dir, "deeprev.kp")
	deepRevSync(t, s, []string{"alpha 10 1 y", "beta 10 1 y", "gamma 10 1 y"}, "-F", db, "-g", "-a", "-NE")

	port := dsyncPeer{Active: []string{
		"alpha 1000000000000000 1 y",    // 16 digits
		"beta 99999999999999999999 1 y", // 20 digits, past 64 bits
		"gamma 99999999999999 1 y",      // 14 digits: still taken
	}}.listen(t)
	out, err := s.TryRun("dsyncgroups", "-h", "127.0.0.1", "-p", strconv.Itoa(port), "-f", db, "-a", "-NE", "-v")
	if err != nil {
		t.Fatalf("dsyncgroups with 16- and 20-digit remote numbers: %v\n%s", err, out)
	}
	for _, g := range []string{"alpha", "beta"} {
		if !strings.Contains(out, "Article number out of range - skipping : "+g) {
			t.Errorf("%s: no \"out of range\" line:\n%s", g, out)
		}
		if ne := deepRevNE(t, db, g); ne != "0000000010" {
			t.Errorf("%s: NE=%s after a remote number past the bound, want it kept at 0000000010", g, ne)
		}
	}
	if ne := deepRevNE(t, db, "gamma"); ne != "99999999999999" {
		t.Errorf("gamma: NE=%s, want 99999999999999\n%s", ne, out)
	}
	// with -D (delete local groups the remote does not list) a group that the remote
	// lists with a bad number is still listed: it must stay (Terra, review of this fix)
	port = dsyncPeer{Active: []string{
		"alpha 1000000000000000 1 y",
		"beta 99999999999999999999 1 y",
		"gamma 99999999999999 1 y",
	}}.listen(t) // a fake peer takes one connection
	out, err = s.TryRun("dsyncgroups", "-h", "127.0.0.1", "-p", strconv.Itoa(port), "-f", db, "-a", "-NE", "-D", "-v")
	if err != nil {
		t.Fatalf("dsyncgroups -D: %v\n%s", err, out)
	}
	for _, g := range []string{"alpha", "beta", "gamma"} {
		if live, _, _ := kpRecords(t, db, g); len(live) != 1 {
			t.Errorf("dsyncgroups -D: %s has %d live records, want 1 (it is listed by the remote)\n%s", g, len(live), out)
		}
	}
	// a local record with such a number (dactive.kp written by something else): skipped
	// with the same message, not a crash (oc-1, review of this fix)
	for _, f := range []string{"NB=0000000001", "NE=1000000000000000", "NX=0000000001"} {
		s.Run(t, "dkp", "-w", db, "delta", f) // dkp -w stores one field per call
	}
	if live, _, _ := kpRecords(t, db, "delta"); len(live) != 1 || !strings.Contains(live[0], "NE=1000000000000000") {
		t.Fatalf("setup: dkp did not write delta with NE=1000000000000000: %q", live)
	}
	port = dsyncPeer{Active: []string{"gamma 99999999999999 1 y"}}.listen(t)
	out, err = s.TryRun("dsyncgroups", "-h", "127.0.0.1", "-p", strconv.Itoa(port), "-f", db, "-a", "-NE", "-v")
	if err != nil {
		t.Fatalf("dsyncgroups with a local NE of 16 digits: %v\n%s", err, out)
	}
	if !strings.Contains(out, "Article number out of range - skipping : delta") {
		t.Errorf("no \"out of range\" line for the local record delta:\n%s", out)
	}
	// with -g a remote line for delta must not make it a new group that is written over
	// its record, and -D must not delete it (Terra, re-review of this fix)
	port = dsyncPeer{Active: []string{"delta 50 1 y", "gamma 99999999999999 1 y"}}.listen(t)
	out, err = s.TryRun("dsyncgroups", "-h", "127.0.0.1", "-p", strconv.Itoa(port), "-f", db, "-g", "-a", "-NE", "-v")
	if err != nil {
		t.Fatalf("dsyncgroups -g with a remote line for delta: %v\n%s", err, out)
	}
	if ne := deepRevNE(t, db, "delta"); ne != "1000000000000000" || strings.Contains(out, "Creating delta") {
		t.Errorf("dsyncgroups -g wrote over the local record of delta: NE=%s\n%s", ne, out)
	}
	port = dsyncPeer{Active: []string{"gamma 99999999999999 1 y"}}.listen(t)
	out, err = s.TryRun("dsyncgroups", "-h", "127.0.0.1", "-p", strconv.Itoa(port), "-f", db, "-a", "-NE", "-D", "-v")
	if err != nil {
		t.Fatalf("dsyncgroups -D: %v\n%s", err, out)
	}
	if live, _, _ := kpRecords(t, db, "delta"); len(live) != 1 {
		t.Errorf("dsyncgroups -D deleted delta, whose record it leaves alone\n%s", out)
	}
	// 4294967296 is 0 once cut to an int (oc-1, review of this fix)
	// (port 1 refuses: the message tells a refused value from a failed connect)
	port = 1
	for _, p := range []string{"10001", "-1", "4294967296", "50x"} {
		if out, err := s.TryRun("dsyncgroups", "-h", "127.0.0.1", "-p", strconv.Itoa(port), "-f", db, "-a", "-NE", "-P", p); err == nil ||
			!strings.Contains(out, "-P must be") {
			t.Errorf("dsyncgroups -P %s was taken:\n%s", p, out)
		}
	}
	// -n, added to the remote number, is checked as a long too, within -10^9..10^9
	for _, n := range []string{"1000000001", "-1000000001", "4294967296", "5x"} {
		if out, err := s.TryRun("dsyncgroups", "-h", "127.0.0.1", "-p", strconv.Itoa(port), "-f", db, "-a", "-NE", "-n", n); err == nil ||
			!strings.Contains(out, "-n must be") {
			t.Errorf("dsyncgroups -n %s was taken:\n%s", n, out)
		}
	}
}

// deepRevField returns field f (NE, NX, ...) of the group's record in the KP database db.
func deepRevField(t *testing.T, db, group, f string) string {
	t.Helper()
	live, _, _ := kpRecords(t, db, group)
	if len(live) != 1 {
		t.Fatalf("%s: %d live records for %s: %q", db, len(live), group, live)
	}
	for _, kv := range strings.Fields(live[0]) {
		if v, ok := strings.CutPrefix(kv, f+"="); ok {
			return v
		}
	}
	t.Fatalf("no %s in %q", f, live[0])
	return ""
}

// TestDeepRevDsyncgroupsAdditionBound (Sol R3, re-review of the number bound): a remote number
// just below 10^14 is taken, but -n added to it could make NE and NX above 10^14, a record the
// next run refuses to load as its own. Such a result is now refused as a remote number that
// high would be: the group keeps its numbers, and the next sync still updates it.
func TestDeepRevDsyncgroupsAdditionBound(t *testing.T) {
	s := NewSite(t, SiteOptions{NoStart: true})
	db := filepath.Join(s.Dir, "deeprev.kp")
	deepRevSync(t, s, []string{"alpha 10 1 y"}, "-F", db, "-g", "-a", "-NE", "-XE")
	if ne, nx := deepRevField(t, db, "alpha", "NE"), deepRevField(t, db, "alpha", "NX"); ne != "0000000010" || nx != "0000000010" {
		t.Fatalf("setup: NE=%s NX=%s, want 0000000010", ne, nx)
	}
	out := deepRevSync(t, s, []string{"alpha 99999999999999 1 y"}, "-f", db, "-a", "-NE", "-XE", "-n", "1000", "-v")
	for _, f := range []string{"NE", "NX"} {
		if v := deepRevField(t, db, "alpha", f); v != "0000000010" {
			t.Errorf("%s=%s after a remote number 10^14-1 and -n 1000, want it kept at 0000000010\n%s", f, v, out)
		}
		if !strings.Contains(out, "Article number out of range - skipping "+f+" : alpha") {
			t.Errorf("no \"out of range\" line for %s:\n%s", f, out)
		}
	}
	// the record still loads: the next sync updates it (a record it could not load would
	// be taken as a new group, -a)
	out = deepRevSync(t, s, []string{"alpha 20 1 y"}, "-f", db, "-a", "-NE", "-v")
	if ne := deepRevField(t, db, "alpha", "NE"); ne != "0000000020" || !strings.Contains(out, "Updating alpha") {
		t.Errorf("NE=%s after the next sync with 20, want 0000000020 by an update of the record\n%s", ne, out)
	}

	// a large negative -n is refused the same way, not written as a negative number
	// (Terra, check of round 2)
	out = deepRevSync(t, s, []string{"alpha 30 1 y"}, "-f", db, "-a", "-NE", "-n", "-1000000000", "-v")
	if ne := deepRevField(t, db, "alpha", "NE"); ne != "0000000020" || !strings.Contains(out, "Article number out of range - skipping NE : alpha") {
		t.Errorf("NE=%s after -n -1000000000, want it kept at 0000000020 with an \"out of range\" line\n%s", ne, out)
	}

	// a refused update of a group whose NE is 0 leaves 0, not 1 (0 counts as 1 only for
	// the percentage); the low number changing in the same run (-NB) writes the record
	s.Run(t, "dkp", "-w", db, "beta", "NB=0000000001")
	s.Run(t, "dkp", "-w", db, "beta", "NE=0000000000")
	out = deepRevSync(t, s, []string{"beta 99999999999999 7 y"}, "-f", db, "-a", "-NE", "-NB", "-n", "1000", "-v")
	if nb := deepRevField(t, db, "beta", "NB"); nb != "0000000007" {
		t.Fatalf("setup: beta NB=%s, want 0000000007 (the record must be written)\n%s", nb, out)
	}
	if ne := deepRevField(t, db, "beta", "NE"); ne != "0000000000" {
		t.Errorf("beta: NE=%s after a refused update, want 0000000000\n%s", ne, out)
	}
}

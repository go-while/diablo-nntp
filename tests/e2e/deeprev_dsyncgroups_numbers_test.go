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

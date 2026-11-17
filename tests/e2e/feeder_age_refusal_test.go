package e2e

import (
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"
)

// feederMapCount returns the number of mappings of pid (lines of /proc/pid/maps).
func feederMapCount(t *testing.T, pid int) int {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "maps"))
	if err != nil {
		t.Fatalf("maps of %d: %v", pid, err)
	}
	return strings.Count(string(b), "\n")
}

// TestFeederAgeRefusalReleasesArticle (deep review r2 H3, D-227): "HEAD|BODY|ARTICLE <id>
// a<N>" for an article older than N seconds is answered "430 Article prohibited". The
// feeder has already opened (mapped) the article at that point; before the fix the refusal
// dropped the pointer without releasing it, so each refused request left one mapping in
// the connection's diablo child (dreaderd passes a reader's own "a<N>" through, so any
// reader could repeat it until the map limit made every fetch on that connection fail).
// The refused requests of all three commands must leave the mapping count flat, and the
// connection must still serve the article without and with a limit it passes.
func TestFeederAgeRefusalReleasesArticle(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("counts mappings in /proc/PID/maps")
	}
	s := NewSite(t, SiteOptions{})
	id := s.Inject(t, "e2e.test", 1)[0]
	// the history keeps the arrival in minutes: two seconds make it older than a1 in any case
	time.Sleep(2 * time.Second)

	before := sitePids(t, s, "diablo")
	p := s.Peer(t)
	defer p.Close()
	p.ExpectCmd(223, "STAT %s", id)
	child := 0
	for _, pid := range sitePids(t, s, "diablo") {
		if !slices.Contains(before, pid) && pid > child {
			child = pid
		}
	}
	if child == 0 {
		t.Fatalf("no diablo child for the peer connection (before %v)", before)
	}

	// one served fetch first, so that anything the first fetch sets up once is in m0
	p.ExpectCmd(220, "ARTICLE %s", id)
	p.ReadDotLines()

	m0 := feederMapCount(t, child)
	const n = 150
	for _, cmd := range []string{"HEAD", "BODY", "ARTICLE"} {
		for i := 0; i < n; i++ {
			if code, line := p.Cmd("%s %s a1", cmd, id); code != 430 {
				t.Fatalf("request %d: %s %s a1: %d %q, want 430", i+1, cmd, id, code, line)
			}
		}
	}
	m1 := feederMapCount(t, child)
	t.Logf("diablo child %d: %d mappings before, %d after %d refused requests", child, m0, m1, 3*n)
	if m1-m0 > 20 {
		t.Errorf("%d refused requests left %d more mappings in the diablo child, want none", 3*n, m1-m0)
	}

	// served fetches on the same connection: no limit, and a limit the article passes
	p.ExpectCmd(220, "ARTICLE %s", id)
	p.ReadDotLines()
	p.ExpectCmd(221, "HEAD %s a999999999", id)
	p.ReadDotLines()
	p.ExpectCmd(222, "BODY %s a999999999", id)
	p.ReadDotLines()
	if m2 := feederMapCount(t, child); m2-m0 > 20 {
		t.Errorf("after the served fetches the diablo child has %d more mappings than before, want none", m2-m0)
	}
}

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

// mapCount returns the number of mappings of pid (lines of /proc/pid/maps).
func mapCount(t *testing.T, pid int) int {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "maps"))
	if err != nil {
		t.Fatalf("maps of %d: %v", pid, err)
	}
	return strings.Count(string(b), "\n")
}

// TestFeederAgeRefusalLeaks (deep review r2 H3): "ARTICLE <id> a<N>" for an article older
// than N seconds is answered "430 Article prohibited", but ArticleOpen() has already mapped
// the article and the refusal drops the pointer without releasing it. Each such request
// leaves one mapping behind in the connection's diablo child; dreaderd passes a reader's
// own "a<N>" through, so any reader can repeat it.
func TestFeederAgeRefusalLeaks(t *testing.T) {
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

	m0 := mapCount(t, child)
	const n = 400
	for i := 0; i < n; i++ {
		if code, line := p.Cmd("ARTICLE %s a1", id); code != 430 {
			t.Fatalf("request %d: ARTICLE %s a1: %d %q, want 430", i+1, id, code, line)
		}
	}
	m1 := mapCount(t, child)
	t.Logf("diablo child %d: %d mappings before, %d after %d refused requests", child, m0, m1, n)
	if m1-m0 > 20 {
		t.Errorf("%d refused requests left %d more mappings in the diablo child, want none", n, m1-m0)
	}
	p.ExpectCmd(220, "ARTICLE %s", id)
	p.ReadDotLines()
}

// TestReaderAgeArgOverridesPolicy (deep review r2 M2): a reader's "ARTICLE <id> a<N>"
// replaces the readerdef's maxagepolicy instead of tightening it, and "a0" sends no limit
// at all, so the reader gets articles older than its retention.
func TestReaderAgeArgOverridesPolicy(t *testing.T) {
	feeds := DefaultNewsfeeds + "label reader\n    inhost 127.0.0.1\n    addgroup *\nend\n"
	feeder := NewSite(t, SiteOptions{Newsfeeds: feeds})
	id := feeder.Inject(t, "e2e.test", 1)[0]
	access := strings.Replace(DefaultReaderAccess, "    post yes\n", "    post yes\n    maxagepolicy 1\n", 1)
	s := NewReaderSite(t, ReaderOptions{SpoolAddr: feeder.Addr, Access: access})
	time.Sleep(2 * time.Second) // older than the policy's 1 s (see TestFeederAgeRefusalLeaks)

	c := s.Reader(t)
	defer c.Close()
	// control: the policy alone refuses it
	if code, line := c.Cmd("ARTICLE %s", id); code != 430 {
		if code == 220 {
			c.ReadDotLines()
		}
		t.Fatalf("ARTICLE %s under maxagepolicy 1: %d %q, want 430", id, code, line)
	}
	for _, opt := range []string{"a0", "a999999999"} {
		code, line := c.Cmd("ARTICLE %s %s", id, opt)
		if code == 220 {
			c.ReadDotLines()
		}
		if code != 430 {
			t.Errorf("ARTICLE %s %s under maxagepolicy 1: %d %q, want 430", id, opt, code, line)
		}
	}
}

package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// rewriteNewsfeeds replaces dnewsfeeds and moves its mtime forward: diablo looks at the
// mtime every 10 seconds, and the rewrite may fall into the same second as the original.
func rewriteNewsfeeds(tb testing.TB, s *Site, content string) {
	tb.Helper()
	s.WriteFile(tb, "dnewsfeeds", content)
	future := time.Now().Add(10 * time.Second)
	if err := os.Chtimes(filepath.Join(s.Dir, "dnewsfeeds"), future, future); err != nil {
		tb.Fatal(err)
	}
}

// hostCacheSize returns the size of dhosts.cache, or -1 when it is missing.
func hostCacheSize(s *Site) int64 {
	st, err := os.Stat(filepath.Join(s.Dir, "dhosts.cache"))
	if err != nil {
		return -1
	}
	return st.Size()
}

// TestHostCacheLastPeerRemoved: when the last inhost is removed from dnewsfeeds, diablo
// rebuilds an empty dhosts.cache and the former peer gets "Transfer permission denied".
// Before 6-CUR-20261024-BWH the empty rebuild never replaced the old cache, so the peer kept
// its cached authorization and got "label missing in dnewsfeeds" instead.
//
// The site starts with a primed cache, as after any earlier run, so this test does not
// depend on TestHostCacheRebuildAfterFreshStart.
func TestHostCacheLastPeerRemoved(t *testing.T) {
	s := NewSite(t, SiteOptions{Setup: PrimeHostCache})
	p := s.Peer(t)
	if !strings.HasPrefix(p.Greeting, "200 ") {
		t.Fatalf("peer greeting %q, want 200", p.Greeting)
	}
	expect(t, p, 205, "QUIT")

	// Keep one label, without an inhost, so dnewsfeeds stays valid.
	rewriteNewsfeeds(t, s, GlobalLabel+"label outgoing\n    addgroup *\nend\n")
	waitFor(t, 30*time.Second, "an empty dhosts.cache after the last inhost was removed", func() bool {
		return hostCacheSize(s) == 0
	})

	p = s.Peer(t)
	if !strings.HasPrefix(p.Greeting, "502 ") || !strings.Contains(p.Greeting, "permission denied") {
		t.Fatalf("greeting of the removed peer %q, want 502 ... Transfer permission denied", p.Greeting)
	}
}

// TestHostCacheRebuildAfterFreshStart: diablo started without a dhosts.cache (a first start,
// or after the cache was deleted) still rebuilds it when dnewsfeeds changes, so a peer added
// later is let in.
//
// In 6-CUR-20261024-BWH it does not: at startup diablo waits for the first rebuild with
// waitpid() but never clears HostCachePid, and the main loop only rebuilds while
// HostCachePid is 0 (util/diablo.c), so the cache stays as built at startup until diablo is
// restarted.
func TestHostCacheRebuildAfterFreshStart(t *testing.T) {
	s := NewSite(t, SiteOptions{Newsfeeds: GlobalLabel + "label outgoing\n    addgroup *\nend\n"})
	p := s.Peer(t)
	if !strings.HasPrefix(p.Greeting, "502 ") {
		t.Fatalf("greeting of a peer not yet listed %q, want 502", p.Greeting)
	}

	rewriteNewsfeeds(t, s, DefaultNewsfeeds)
	waitFor(t, 30*time.Second, "dhosts.cache to list the peer added to dnewsfeeds", func() bool {
		return hostCacheSize(s) > 0
	})

	p = s.Peer(t)
	if !strings.HasPrefix(p.Greeting, "200 ") {
		t.Fatalf("greeting of the added peer %q, want 200", p.Greeting)
	}
}

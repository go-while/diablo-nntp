package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestDeepRevDprimehostcacheTwoArguments (L7-01): "dprimehostcache -prime hostcache
// diablo.hosts" (both file arguments, as the usage and the manual page give them) stored the
// second argument as the hosts file but did not step over it, so the next round of the
// argument loop took it as the cache file too: the binary host cache was then renamed over
// the administrator's diablo.hosts. With one argument, or none, it worked.
//
// After the command diablo.hosts must be unchanged and the cache must be in the file named
// first.
func TestDeepRevDprimehostcacheTwoArguments(t *testing.T) {
	const hosts = "127.0.0.9\tpeer\n127.0.0.10\tpeer\n"
	s := NewSite(t, SiteOptions{NoStart: true, Files: map[string]string{"diablo.hosts": hosts}})
	cache := filepath.Join(s.Dir, "hostcache.test")
	hostsFile := filepath.Join(s.Dir, "diablo.hosts")

	out := s.Run(t, "dprimehostcache", "-prime", cache, hostsFile)

	if got := s.ReadFile(t, "diablo.hosts"); got != hosts {
		t.Errorf("diablo.hosts was changed by dprimehostcache -prime <cache> <hosts>: now %d bytes, starts %q\n%s",
			len(got), got[:min(len(got), 40)], out)
	}
	if _, err := os.Stat(cache); err != nil {
		t.Fatalf("the cache file named on the command line was not written: %v\n%s", err, out)
	}
	dump := s.Run(t, "dprimehostcache", "-dump", cache)
	for _, want := range []string{"127.0.0.9 label=peer", "127.0.0.10 label=peer"} {
		if !strings.Contains(dump, want) {
			t.Errorf("dprimehostcache -dump %s has no %q:\n%s", cache, want, dump)
		}
	}
}

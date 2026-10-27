package e2e

import (
	"strings"
	"testing"
)

// hostCacheDump primes the host cache of a stopped site and returns dprimehostcache -dump.
func hostCacheDump(t *testing.T, feeds string, hosts string) string {
	t.Helper()
	files := map[string]string{}
	if hosts != "" {
		files["diablo.hosts"] = hosts
	}
	s := NewSite(t, SiteOptions{NoStart: true, Newsfeeds: feeds, Files: files})
	s.Run(t, "dprimehostcache", "-prime")
	return s.Run(t, "dprimehostcache", "-dump")
}

// TestHostCacheEndEndsLabel (D-60): an inhost line after the `end` of a label belongs to no
// label and must not be authorised under the previous one. Fails on main: the host cache
// listed 127.0.0.3 under label peer.
func TestHostCacheEndEndsLabel(t *testing.T) {
	dump := hostCacheDump(t, GlobalLabel+"label peer\n    inhost "+PeerIP+"\n    addgroup *\nend\n    inhost 127.0.0.3\nlabel other\n    addgroup *\nend\n", "")
	if !strings.Contains(dump, PeerIP+" label=peer") {
		t.Errorf("the inhost inside the label is missing from the host cache:\n%s", dump)
	}
	if strings.Contains(dump, "127.0.0.3") {
		t.Errorf("an inhost line after `end` was entered into the host cache:\n%s", dump)
	}
}

// TestHostCacheDiabloHostsForms (D-61): diablo.hosts takes an IPv6 address, which holds colons,
// and still the older "host: label" and "host:label" forms. Fails on main: 2001:db8::25 became
// the host "2001" with the label "db8".
func TestHostCacheDiabloHostsForms(t *testing.T) {
	dump := hostCacheDump(t, GlobalLabel+"label v6\n    addgroup *\nend\nlabel colon\n    addgroup *\nend\nlabel old\n    addgroup *\nend\n",
		"2001:db8::25\tv6\n127.0.0.9: colon\n127.0.0.10:old\n")
	for _, want := range []string{"2001:db8::25 label=v6", "127.0.0.9 label=colon", "127.0.0.10 label=old"} {
		if !strings.Contains(dump, want) {
			t.Errorf("host cache has no %q:\n%s", want, dump)
		}
	}
	if strings.Contains(dump, "label=db8") || strings.Contains(dump, "2001 label") {
		t.Errorf("the IPv6 address was split at its colons:\n%s", dump)
	}
}

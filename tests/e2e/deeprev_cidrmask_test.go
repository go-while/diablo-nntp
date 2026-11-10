package e2e

import (
	"strings"
	"testing"
	"time"

	testkit "github.com/go-while/nntp-testkit"
)

// TestDeepRevCidrBadMask (L8-04): CidrMatch() (lib/subs.c) read the prefix length of an
// "addr/nn" pattern with atoi() and shifted by 32-nn. A length that is no length ("/", "/33",
// "/64": a typo for /3, /24, ...) gave a mask without any of the 32 address bits, so the
// pattern matched every client: "access 10.9.9.0/33 rp" in dreader.access gave the whole
// Internet the readerdef meant for one network. The same function matches the address
// patterns of diablo.hosts and dnewsfeeds.
//
// A pattern with such a mask must match no address (and be logged); valid masks, /0 included,
// match as before.
func TestDeepRevCidrBadMask(t *testing.T) {
	access := func(pattern string) string {
		return `vserverdef DEFAULT
    hostname ` + PathHost + `
end
readerdef rp
    read yes
    post yes
end
access ` + pattern + ` rp
`
	}
	greeting := func(t *testing.T, pattern string) (*Site, string) {
		s := NewReaderSite(t, ReaderOptions{SpoolAddr: "127.0.0.1:1", Access: access(pattern), NoStart: true})
		startReaderNoWait(t, s)
		var g string
		testkit.WaitFor(t, 10*time.Second, "dreaderd to answer a client", func() bool {
			g = readerGreeting(s.Addr)
			return g != ""
		})
		return s, strings.TrimSpace(g)
	}

	// The test's client comes from 127.0.0.1, which is not in 10.9.9.0/anything but /0.
	for _, pattern := range []string{"10.9.9.0/33", "10.9.9.0/64", "10.9.9.0/", "10.9.9.0/255.255.255.256"} {
		t.Run("bad_"+strings.ReplaceAll(pattern, "/", "_"), func(t *testing.T) {
			s, g := greeting(t, pattern)
			if strings.HasPrefix(g, "200") || strings.HasPrefix(g, "201") {
				t.Errorf("access %s gave service to 127.0.0.1: %q, want a refusal", pattern, g)
			}
			if log := generalLog(s); !strings.Contains(log, "Invalid IPv4 mask: "+pattern) {
				t.Errorf("access %s: no \"Invalid IPv4 mask\" line in the log:\n%s", pattern, log)
			}
		})
	}
	// 0377.0377.0377.0377 is the all-ones mask in inet_addr()'s octal spelling, as valid
	// as 255.255.255.255.
	for _, pattern := range []string{"127.0.0.0/8", "127.0.0.1/32", "0.0.0.0/0", "127.0.0.0/255.0.0.0", "10.9.9.0/24",
		"127.0.0.1/255.255.255.255", "127.0.0.1/0377.0377.0377.0377", "127.0.0.0/0.0.0.0"} {
		t.Run("good_"+strings.ReplaceAll(pattern, "/", "_"), func(t *testing.T) {
			s, g := greeting(t, pattern)
			served := strings.HasPrefix(g, "200") || strings.HasPrefix(g, "201")
			if want := pattern != "10.9.9.0/24"; served != want {
				t.Errorf("access %s: greeting %q for 127.0.0.1, served=%v, want %v", pattern, g, served, want)
			}
			if log := generalLog(s); strings.Contains(log, "Invalid IPv4 mask") {
				t.Errorf("access %s: a valid mask was logged as invalid:\n%s", pattern, log)
			}
		})
	}
}

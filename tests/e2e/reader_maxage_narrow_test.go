package e2e

// D-230 (deep review round 2): a reader's own "ARTICLE <id> a<N>" replaced the readerdef's
// maxagepolicy instead of narrowing it, and "a0" sent no limit at all, so a reader could get
// articles older than its readerdef allows. The argument may now only narrow the policy.

import (
	"testing"
	"time"

	"github.com/go-while/nntp-testkit/client"
)

// TestReaderAgeArgNarrowsPolicy: dreaderd's spool is a real diablo (which applies the age); one
// article, fetched at least 2 s after it was stored. Readerdef "short" (maxagepolicy 1, from 127.0.0.1): the article is refused
// with and without a0, a999999999, a99999999999 (larger than an int) or a non-number, and HEAD
// with a0 too. Readerdef
// "long" (maxagepolicy 3600, from 127.0.0.3): served, also with a0 and a999999999 (the policy
// stays), refused with a1 (narrower). Readerdef "none" (no policy, from 127.0.0.4): served,
// also with a0, refused with a1. Fails before the fix: a0 and a999999999 were served to "short".
func TestReaderAgeArgNarrowsPolicy(t *testing.T) {
	feeds := DefaultNewsfeeds + "label reader\n    inhost 127.0.0.1\n    addgroup *\nend\n"
	feeder := NewSite(t, SiteOptions{Newsfeeds: feeds})
	id := feeder.Inject(t, "e2e.test", 1)[0]
	access := `vserverdef DEFAULT
    hostname ` + PathHost + `
end
readerdef short
    read yes
    maxagepolicy 1
end
readerdef long
    read yes
    maxagepolicy 3600
end
readerdef none
    read yes
end
readerdef feed
    feed yes
end
access 127.0.0.1 short
access 127.0.0.3 long
access 127.0.0.4 none
access ` + PeerIP + ` feed
`
	s := NewReaderSite(t, ReaderOptions{SpoolAddr: feeder.Addr, Access: access})
	// the feeder's history keeps the arrival in minutes: two seconds make the article older
	// than a1 in any case
	time.Sleep(2 * time.Second)

	check := func(c *client.Conn, who, opt string, want int) {
		t.Helper()
		cmd := "ARTICLE " + id
		if opt != "" {
			cmd += " " + opt
		}
		code, line := c.Cmd("%s", cmd)
		if code == 220 {
			c.ReadDotLines()
		}
		if code != want {
			t.Errorf("%s: %s: %d %q, want %d", who, cmd, code, line, want)
		}
	}

	short := s.Reader(t)
	defer short.Close()
	for _, opt := range []string{"", "a0", "a999999999", "a99999999999", "a-5", "ax", "a"} {
		check(short, "maxagepolicy 1", opt, 430)
	}
	if code, line := short.Cmd("HEAD %s a0", id); code != 430 {
		if code == 221 {
			short.ReadDotLines()
		}
		t.Errorf("maxagepolicy 1: HEAD %s a0: %d %q, want 430", id, code, line)
	}

	long := s.dialFrom(t, "127.0.0.3")
	defer long.Close()
	for _, opt := range []string{"", "a0", "a999999999", "ax"} {
		check(long, "maxagepolicy 3600", opt, 220)
	}
	check(long, "maxagepolicy 3600", "a1", 430)

	none := s.dialFrom(t, "127.0.0.4")
	defer none.Close()
	for _, opt := range []string{"", "a0", "a999999999"} {
		check(none, "no maxagepolicy", opt, 220)
	}
	check(none, "no maxagepolicy", "a1", 430)
}

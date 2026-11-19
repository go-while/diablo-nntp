package e2e

import (
	"fmt"
	"testing"
	"time"
)

// TestNewsfeedsReloadOnIdleOffers: a peer connection that only CHECKs, and gets every offer
// refused, takes a change of dnewsfeeds too. The connection's process loaded its label at
// connect and reloaded only when it received an article (LoadArticle(), FeedAdd()), so with
// all offers refused it kept the rules of its connect time for as long as the peer stayed:
// an offerfilter removed from the label went on refusing. The process now looks at
// dnewsfeeds (once a minute) before it answers a command. Before the fix the second CHECK
// below is answered 438 as the first one.
// Takes up to a minute and a quarter: dnewsfeeds is checked once a minute, and a file
// younger than 10 s is left alone for that minute.
func TestNewsfeedsReloadOnIdleOffers(t *testing.T) {
	cfg := func(filter string) string {
		return GlobalLabel + "label peer\n    inhost " + PeerIP + "\n    addgroup *\n" + filter + "end\n"
	}
	id := func(kind string) string {
		return fmt.Sprintf("<%s.%d@%s>", kind, time.Now().UnixNano(), PathHost)
	}
	s := NewSite(t, SiteOptions{Newsfeeds: cfg("    offerfilter <blocked.*\n")})

	p := s.Peer(t) // its process loads its label now; no article will reach it
	defer p.ExpectCmd(205, "QUIT")
	p.ExpectCmd(438, "CHECK %s", id("blocked"))
	p.ExpectCmd(238, "CHECK %s", id("other"))

	// diablo notices a change by the file's modification time in whole seconds
	time.Sleep(1100 * time.Millisecond)
	s.WriteFile(t, "dnewsfeeds", cfg(""))
	written := time.Now()

	// the first check comes after a minute boundary that is 11 s or more behind the write
	boundary := written.Truncate(time.Minute).Add(time.Minute)
	if boundary.Sub(written) < 11*time.Second {
		boundary = boundary.Add(time.Minute)
	}
	sleepUntilWall(t, boundary.Add(300*time.Millisecond))

	p.ExpectCmd(238, "CHECK %s", id("blocked"))
}

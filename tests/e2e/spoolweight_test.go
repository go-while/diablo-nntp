package e2e

import (
	"github.com/go-while/nntp-testkit/client"
	"testing"
)

// TestWeightedAllocationWithZeroWeights (D-64): `allocstrat weighted` with every spool at weight 0
// (what a partition under 1 GiB gets by default) divides by zero. Fails on main: diablo dies with
// SIGFPE at start. Now the articles are spread evenly.
func TestWeightedAllocationWithZeroWeights(t *testing.T) {
	p0, p1 := t.TempDir(), t.TempDir()
	s := NewSite(t, SiteOptions{SpoolCtl: `spool 00
    path ` + p0 + `
    minfree 1m
    weight 0
end
spool 01
    path ` + p1 + `
    minfree 1m
    weight 0
end
metaspool trad
    allocstrat weighted
    spool 00
    spool 01
end
expire * trad
`})
	p := s.Peer(t)
	for i := 0; i < 3; i++ {
		id := msgID(t)
		p.ExpectCmd(335, "IHAVE %s", id)
		p.MustSend(client.Stuff(newArticle(id, "e2e.test", "weights", "body")))
		p.ExpectStatus(235, "IHAVE")
		p.ExpectCmd(223, "STAT %s", id)
	}
	p.ExpectCmd(205, "QUIT")
}

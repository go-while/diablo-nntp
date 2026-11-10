package e2e

import (
	"testing"

	"github.com/go-while/nntp-testkit/client"
)

// TestDeepRevSpoolCtlMaxcross (L2-03): a metaspool's "maxcross N" in dspool.ctl limits it to
// articles posted to at most N newsgroups (samples/dspool.ctl: "maxcross 10"). The value was
// read with the parser for sizes in KB, which multiplies by 1024, so "maxcross 2" meant 2048
// and never excluded anything.
//
// The first metaspool takes articles with up to 2 groups and does not store them
// ("dontstore"); everything else is stored by the second. An article in 5 groups must be
// stored (STAT 223); one in a single group must not (STAT 430).
func TestDeepRevSpoolCtlMaxcross(t *testing.T) {
	s := NewSite(t, SiteOptions{SpoolCtl: `spool 00
    minfree 1m
end
metaspool few
    dontstore yes
    maxcross 2
end
metaspool trad
    spool 00
end
expire * few
expire * trad
`})
	p := s.Peer(t)
	// feed sends an article with IHAVE and returns its id and the final reply.
	feed := func(groups string) (id string, code int, line string) {
		t.Helper()
		id = msgID(t)
		p.ExpectCmd(335, "IHAVE %s", id)
		p.MustSend(client.Stuff(newArticle(id, groups, "maxcross", "body")))
		code, line, err := p.ReadStatus()
		if err != nil {
			t.Fatalf("IHAVE %s: %v (%q)", id, err, line)
		}
		return id, code, line
	}
	one, _, _ := feed("e2e.test")
	five, code, line := feed("e2e.a,e2e.b,e2e.c,e2e.d,e2e.e")
	if code != 235 {
		t.Errorf("IHAVE of the article in 5 groups: %q, want 235", line)
	}

	if code, line := p.Cmd("STAT %s", one); code != 430 {
		t.Errorf("an article in 1 group matches the \"maxcross 2\" metaspool (dontstore) but was stored: STAT gives %q, want 430", line)
	}
	if code, line := p.Cmd("STAT %s", five); code != 223 {
		t.Errorf("an article in 5 groups was taken by the metaspool with \"maxcross 2\" (dontstore): STAT gives %q, want 223", line)
	}
	p.ExpectCmd(205, "QUIT")
}

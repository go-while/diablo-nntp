package e2e

import (
	"strings"
	"testing"

	"github.com/go-while/nntp-testkit/client"
)

// deepRevXrefSite is a feeder that numbers the articles itself: "active on" with groups
// added to dactive.kp as they are seen, so LoadArticle() calls GenerateXRef().
func deepRevXrefSite(t *testing.T, conf map[string]string) *Site {
	t.Helper()
	c := map[string]string{"active": "on", "feederautoaddtoactive": "on"}
	for k, v := range conf {
		c[k] = v
	}
	// diablo opens dactive.kp, it does not create it
	return NewSite(t, SiteOptions{Config: c, Files: map[string]string{"dactive.kp": ""}})
}

// deepRevXref feeds the article with IHAVE and returns the value of the Xref: header that
// diablo stored for it.
func deepRevXref(t *testing.T, p *client.Conn, id string, art []string) string {
	t.Helper()
	p.ExpectCmd(335, "IHAVE %s", id)
	p.MustSend(client.Stuff(art))
	p.ExpectStatus(235, "IHAVE "+id)
	p.ExpectCmd(221, "HEAD %s", id)
	return headerValue(p.ReadDotLines(), "Xref")
}

// TestDeepRevXrefDuplicateGroup (L1-13): GenerateXRef() walked the Newsgroups: list and gave
// every entry an article number, also a group that was already in the list. An article with
// "Newsgroups: a,a,a" was stored with "Xref: host a:1 a:2 a:3": the same group several times
// with different numbers, and each repeat used up a number of the group. A group gets one
// Xref entry and one number per article.
func TestDeepRevXrefDuplicateGroup(t *testing.T) {
	s := deepRevXrefSite(t, nil)
	p := s.Peer(t)

	id1 := msgID(t)
	x1 := deepRevXref(t, p, id1, newArticle(id1, "e2e.test,e2e.other, e2e.test,e2e.test", "duplicate group", "body"))
	if want := PathHost + " e2e.test:1 e2e.other:1"; x1 != want {
		t.Errorf("Xref of an article with Newsgroups: e2e.test,e2e.other, e2e.test,e2e.test: %q, want %q", x1, want)
	}

	id2 := msgID(t)
	x2 := deepRevXref(t, p, id2, newArticle(id2, "e2e.test", "next article", "body"))
	if want := PathHost + " e2e.test:2"; x2 != want {
		t.Errorf("Xref of the next article in e2e.test: %q, want %q (the repeats used up article numbers)", x2, want)
	}
	p.ExpectCmd(205, "QUIT")
	if strings.Count(x1, "e2e.test:") != 1 {
		t.Logf("e2e.test is in the first Xref %d times", strings.Count(x1, "e2e.test:"))
	}
}

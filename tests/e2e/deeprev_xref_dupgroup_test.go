package e2e

import (
	"strconv"
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

// TestDeepRevXrefDuplicateGroupLongList: the same rule in a long list. From the 33rd name on
// GenerateXRef() looks names up in a hash table instead of comparing with every name before
// (a list of thousands of names cost a scan of the whole list per name). A repeat is found
// whether the first one was among the first 32 names or after them.
func TestDeepRevXrefDuplicateGroupLongList(t *testing.T) {
	s := deepRevXrefSite(t, nil)
	p := s.Peer(t)

	ng := []string{"e2e.nogroup.a", "e2e.test"}
	for i := 0; i < 40; i++ {
		ng = append(ng, "e2e.nogroup.f"+strconv.Itoa(i))
	}
	// e2e.test again (first seen among the first 32), e2e.other twice (first seen after them)
	ng = append(ng, "e2e.test", "e2e.other", "e2e.nogroup.f7", "e2e.other", "e2e.test")

	id := msgID(t)
	x := deepRevXref(t, p, id, newArticle(id, strings.Join(ng, ","), "long list", "body"))
	seen := map[string]int{}
	for _, f := range strings.Fields(x)[1:] {
		name, _, _ := strings.Cut(f, ":")
		seen[name]++
	}
	for name, n := range seen {
		if n != 1 {
			t.Errorf("%s is in the Xref %d times", name, n)
		}
	}
	if seen["e2e.test"] != 1 || seen["e2e.other"] != 1 {
		t.Errorf("Xref of an article with %d names in Newsgroups:, e2e.test three times and e2e.other twice: %q",
			len(ng), x)
	}
	p.ExpectCmd(205, "QUIT")
}

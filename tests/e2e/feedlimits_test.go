package e2e

import (
	"fmt"
	"slices"
	"strings"
	"testing"
)

// limitLabels is a dnewsfeeds with one label per limit option, all accepting every group.
// Sizes are bytes, with 'k' for 1024.
var limitLabels = GlobalLabel + `label maxsize
    hostname 127.0.0.1
    addgroup *
    maxsize 2k
end
label minsize
    hostname 127.0.0.1
    addgroup *
    minsize 1k
end
label maxcross
    hostname 127.0.0.1
    addgroup *
    maxcross 3
end
label mincross
    hostname 127.0.0.1
    addgroup *
    mincross 2
end
label maxpath
    hostname 127.0.0.1
    addgroup *
    maxpath 3
end
label minpath
    hostname 127.0.0.1
    addgroup *
    minpath 2
end
`

// matched returns the labels dfeedtest says an article with these properties goes to.
func matched(t *testing.T, s *Site, groups string, size int, path string) []string {
	t.Helper()
	out := s.Run(t, "dfeedtest", "-n", groups, "-s", fmt.Sprint(size), "-p", path)
	var labels []string
	for _, l := range strings.Split(out, "\n") {
		if v, ok := strings.CutPrefix(l, "Matched: "); ok {
			labels = append(labels, strings.TrimSpace(v))
		}
	}
	slices.Sort(labels)
	return labels
}

// TestDnewsfeedsLimitsDfeedtest: the dnewsfeeds options maxsize, minsize, maxcross, mincross,
// maxpath and minpath, with the edges of each range, as dfeedtest (which loads dnewsfeeds the
// way diablo does) reports them. A label with a limit lets an article through only inside it;
// 'k' means 1024 bytes; sizes are bytes of the whole article; a path is the Path: header
// value, counted by its '!'-separated elements; crossposts are the groups of Newsgroups:.
// minsize is exclusive (minsize 1k feeds articles larger than 1k) while maxsize is inclusive
// (maxsize 2k feeds up to and including 2k); both are pinned below.
func TestDnewsfeedsLimitsDfeedtest(t *testing.T) {
	s := NewSite(t, SiteOptions{Newsfeeds: limitLabels, NoStart: true})
	all := []string{"maxcross", "maxpath", "maxsize", "mincross", "minpath", "minsize"}
	without := func(drop ...string) []string {
		var l []string
		for _, n := range all {
			if !slices.Contains(drop, n) {
				l = append(l, n)
			}
		}
		return l
	}
	const (
		g1 = "e2e.a"
		g2 = "e2e.a,e2e.b"
		g3 = "e2e.a,e2e.b,e2e.c"
		g4 = "e2e.a,e2e.b,e2e.c,e2e.d"
		p2 = "x!y"
		p3 = "x!y!z"
		p4 = "w!x!y!z"
	)
	for _, c := range []struct {
		name   string
		groups string
		size   int
		path   string
		want   []string
	}{
		// All inside every range: size 1500, 2 groups, 2 path elements.
		{"inside", g2, 1500, p2, all},
		// maxsize 2k = 2048: 2048 is fed, 2049 is not.
		{"size at maxsize", g2, 2048, p2, all},
		{"size over maxsize", g2, 2049, p2, without("maxsize")},
		// minsize 1k = 1024: 1025 is fed, 1023 is not.
		{"size over minsize", g2, 1025, p2, all},
		{"size under minsize", g2, 1023, p2, without("minsize")},
		// Pinned as it is (existing configs depend on it): minsize is exclusive, so exactly
		// 1024 is not fed, unlike exactly maxsize.
		{"size at minsize", g2, 1024, p2, without("minsize")},
		{"size 0", g2, 0, p2, without("minsize")},
		// maxcross 3: three groups are fed, four are not. mincross 2: one is not.
		{"3 groups", g3, 1500, p2, all},
		{"4 groups", g4, 1500, p2, without("maxcross")},
		{"1 group", g1, 1500, p2, without("mincross")},
		// maxpath 3: three elements are fed, four are not. minpath 2: one is not.
		{"3 path elements", g2, 1500, p3, all},
		{"4 path elements", g2, 1500, p4, without("maxpath")},
		{"1 path element", g2, 1500, "x", without("minpath")},
		// Several limits break at once: the max limits are exceeded, the min limits are met.
		{"everything out", g4, 5000, p4, []string{"mincross", "minpath", "minsize"}},
	} {
		t.Run(c.name, func(t *testing.T) {
			got := matched(t, s, c.groups, c.size, c.path)
			if !slices.Equal(got, c.want) {
				t.Errorf("groups %q size %d path %q: matched %v, want %v", c.groups, c.size, c.path, got, c.want)
			}
		})
	}
}

// liveCase is one article sent to TestDnewsfeedsLimitsLive; in says whether the label's
// queue should get it.
type liveCase struct {
	name   string
	groups string
	path   string // the Path: header value; "" is newArticle's "e2e.peer!not-for-mail" (2 elements)
	body   int
	in     bool
}

// TestDnewsfeedsLimitsLive: the same limits on a running diablo: articles with a size, a
// number of groups or a path outside a label's range do not reach that label's queue.
// Sizes keep well away from the edges (the edges are in TestDnewsfeedsLimitsDfeedtest). The
// article that is queued comes last, so that once its entry is there, the ones before it
// have been decided.
func TestDnewsfeedsLimitsLive(t *testing.T) {
	const g5 = "e2e.test,e2e.other,e2e.c,e2e.d,e2e.e"
	for _, c := range []struct {
		label string
		cases []liveCase
	}{
		{"maxsize", []liveCase{{"large", "e2e.test", "", 4000, false}, {"small", "e2e.test", "", 300, true}}},
		{"minsize", []liveCase{{"small", "e2e.test", "", 100, false}, {"large", "e2e.test", "", 2000, true}}},
		{"maxcross", []liveCase{{"5 groups", g5, "", 300, false}, {"2 groups", "e2e.test,e2e.other", "", 300, true}}},
		{"mincross", []liveCase{{"1 group", "e2e.test", "", 300, false}, {"2 groups", "e2e.test,e2e.other", "", 300, true}}},
		{"maxpath", []liveCase{{"5 elements", "e2e.test", "h1!h2!h3!e2e.peer!not-for-mail", 300, false}, {"2 elements", "e2e.test", "", 300, true}}},
		{"minpath", []liveCase{{"1 element", "e2e.test", "not-for-mail", 300, false}, {"3 elements", "e2e.test", "h1!e2e.peer!not-for-mail", 300, true}}},
	} {
		t.Run(c.label, func(t *testing.T) {
			s := NewSite(t, SiteOptions{Newsfeeds: GlobalLabel + `label peer
    inhost ` + PeerIP + `
    alias e2e.peer
    addgroup *
end
` + labelBlock(t, c.label)})
			p := s.Peer(t)
			ids := map[bool][]string{}
			for _, k := range c.cases {
				id := msgID(t)
				a := newArticle(id, k.groups, c.label+" "+k.name, strings.Repeat("x", k.body))
				if k.path != "" {
					a[0] = "Path: " + k.path
				}
				expect(t, p, 335, "IHAVE %s", id)
				send(t, p, wire(a))
				status(t, p, 235, "IHAVE "+id)
				ids[k.in] = append(ids[k.in], id)
			}
			expect(t, p, 205, "QUIT")

			q := s.waitQueue(t, c.label, len(ids[true]))
			joined := strings.Join(q, "\n")
			for _, id := range ids[true] {
				if !strings.Contains(joined, id) {
					t.Errorf("dqueue/%s = %q, want %s", c.label, q, id)
				}
			}
			for _, id := range ids[false] {
				if strings.Contains(joined, id) {
					t.Errorf("dqueue/%s = %q, must not have %s (outside the limit)", c.label, q, id)
				}
			}
		})
	}
}

// labelBlock returns the label of limitLabels with this name.
func labelBlock(t *testing.T, name string) string {
	t.Helper()
	_, rest, ok := strings.Cut(limitLabels, "label "+name+"\n")
	if !ok {
		t.Fatalf("no label %s in limitLabels", name)
	}
	body, _, _ := strings.Cut(rest, "end\n")
	return "label " + name + "\n" + body + "end\n"
}

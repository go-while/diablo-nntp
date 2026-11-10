package e2e

import (
	"testing"
)

// TestDeepRevArttypesNone (L2-04): the article type "none" is documented as "no articles"
// (samples/dnewsfeeds), and an unknown type name is logged and also becomes "none". Its value
// is 0, and the match "(type & wanted) == wanted" is true for every article when wanted is 0:
// a feed with "arttypes none" (or with a misspelt type) got everything.
//
// Two outgoing labels get the same articles; the one with "arttypes none" must stay empty.
func TestDeepRevArttypesNone(t *testing.T) {
	for _, value := range []string{"none", "nosuchtype"} {
		t.Run(value, func(t *testing.T) {
			feeds := GlobalLabel + `label peer
    inhost ` + PeerIP + `
    alias e2e.peer
    addgroup *
end
label all
    hostname 127.0.0.1
    alias e2e.all
    addgroup *
end
label ` + OutLabel + `
    hostname 127.0.0.1
    alias e2e.fake
    addgroup *
    arttypes ` + value + `
end
`
			s := NewSite(t, SiteOptions{Newsfeeds: feeds})
			ids := s.Inject(t, "e2e.test", 2)
			// "all" is written by the same call that decides about the other label.
			s.waitQueue(t, "all", len(ids))
			if q := s.queue(t, OutLabel); len(q) != 0 {
				t.Errorf("the feed with \"arttypes %s\" got %d of %d articles, want none: %q", value, len(q), len(ids), q)
			}
		})
	}
}

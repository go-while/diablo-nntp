package e2e

import (
	"strings"
	"testing"
	"time"
)

// TestDeepRevHashfeedBadOffset (NEW-4, an addition to L2-02): the ":offset" of a hashfeed
// entry selects 4 of the 16 bytes of the Message-ID's MD5, so it is 0..12. The per-entry
// parser refused a larger one, but DiabHashFeedParse() (lib/subs.c) ignored that and linked
// the entry. HM_GetHashInt() refuses such an offset itself and takes 0 for the hash, so the
// entry silently selected articles by a hash of 0. The offset was also stored in a char
// before the check, so 256 became 0 and passed (Terra, second review), and junk after the
// number was ignored.
//
// Such a line must be reported, and the largest valid offset must still be taken.
func TestDeepRevHashfeedBadOffset(t *testing.T) {
	for _, tc := range []struct {
		value string
		bad   bool
	}{{"1/2:13", true}, {"1/2:50", true}, {"1/2:256", true}, {"1/2:4x", true}, {"1/2:12", false}} {
		t.Run(strings.NewReplacer("/", "_", ":", "_").Replace(tc.value), func(t *testing.T) {
			feeds := GlobalLabel + `label peer
    inhost ` + PeerIP + `
    alias e2e.peer
    addgroup *
end
label ` + OutLabel + `
    hostname 127.0.0.1
    alias e2e.fake
    addgroup *
    hashfeed ` + tc.value + `
end
`
			s := NewSite(t, SiteOptions{Newsfeeds: feeds})
			deepRevKillChildren(t, s, "diablo")
			s.Inject(t, "e2e.test", 1)
			time.Sleep(time.Second) // the master handles the article's SOUT line right after the 235
			if out, err := s.TryRun("dicmd", "status"); err != nil {
				t.Fatalf("hashfeed %s: dicmd status after an article: %v\n%s", tc.value, err, out)
			}
			if got := strings.Contains(s.logOutput(), "bad hashfeed entry"); got != tc.bad {
				t.Errorf("hashfeed %s: \"bad hashfeed entry\" in the log: %v, want %v:\n%s", tc.value, got, tc.bad, s.logOutput())
			}
		})
	}
}

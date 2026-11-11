package e2e

import (
	"strings"
	"testing"
	"time"
)

// TestDeepRevHashfeedBadValue (L2-02): a dnewsfeeds "hashfeed" value is parsed by
// DiabHashFeedParse() (lib/subs.c), which ignored the result of the per-entry parser and
// linked every entry, also one with no modulus ("1-2": the "/n" forgotten) or modulus 0.
// The diablo master then computed "hash % 0" for the first article it distributed
// (lib/hashfeed.c, HM_CheckForMatch_PC()): SIGFPE on x86, and again after every restart
// until the line was corrected. Where the division does not trap (arm64) the entry silently
// selected articles by a meaningless rule.
//
// With such a line diablo must report it, the master must stay up when an article comes in,
// and the feed must get no article: a hashfeed that cannot be read matches nothing (Terra,
// second review: a NULL list means "no hashfeed" to every caller, so a typo fed the peer
// everything). "1/1" is the control: it matches every article.
func TestDeepRevHashfeedBadValue(t *testing.T) {
	for _, value := range []string{"1-2", "1-2/0", "abc", "1x/2", "1/2,1-2", "1/1"} {
		t.Run(strings.ReplaceAll(value, "/", "_"), func(t *testing.T) {
			feeds := GlobalLabel + `label peer
    inhost ` + PeerIP + `
    alias e2e.peer
    addgroup *
end
label ` + OutLabel + `
    hostname 127.0.0.1
    alias e2e.fake
    addgroup *
    hashfeed ` + value + `
end
`
			s := NewSite(t, SiteOptions{Newsfeeds: feeds})
			deepRevKillChildren(t, s, "diablo")
			ids := s.Inject(t, "e2e.test", 1)
			bad := value != "1/1"

			// The master handles the article's SOUT line right after the 235.
			limit := time.Now().Add(5 * time.Second)
			for time.Now().Before(limit) {
				select {
				case <-s.Proc.Done():
					t.Fatalf("hashfeed %s: the diablo master died when it distributed an article:\n%s", value, s.logOutput())
				default:
				}
				time.Sleep(100 * time.Millisecond)
			}
			if out, err := s.TryRun("dicmd", "status"); err != nil {
				t.Fatalf("hashfeed %s: dicmd status after an article: %v\n%s", value, err, out)
			}
			if got := strings.Contains(s.logOutput(), "bad hashfeed entry"); got != bad {
				t.Errorf("hashfeed %s: \"bad hashfeed entry\" in the log: %v, want %v:\n%s", value, got, bad, s.logOutput())
			}
			q := s.queue(t, OutLabel)
			if got := len(deepRevMissing(q, ids)) == 0; got == bad {
				t.Errorf("hashfeed %s: article queued for %s: %v, want %v; queue: %q", value, OutLabel, got, !bad, q)
			}
		})
	}
}

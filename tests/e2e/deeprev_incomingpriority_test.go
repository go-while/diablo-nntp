package e2e

import (
	"strings"
	"testing"
)

// TestDeepRevIncomingPriority (L2-12): the dnewsfeeds option "incomingpriority" is the nice
// value of the label's incoming sessions, 0..20 (samples/dnewsfeeds), but it was read with
// enabled(), the yes/no parser, which knows only a leading "y", "on" or "1": 10 and 15 gave
// 1, and 5 or 20 gave 0. The number must be taken as it is.
func TestDeepRevIncomingPriority(t *testing.T) {
	for _, c := range []struct{ value, want string }{
		{"10", "10"},
		{"5", "5"},
		{"20", "20"},
		{"0", "0"},
	} {
		t.Run(c.value, func(t *testing.T) {
			s := NewSite(t, SiteOptions{Newsfeeds: GlobalLabel + "label peer\n    inhost " + PeerIP +
				"\n    addgroup *\n    incomingpriority " + c.value + "\nend\n"})
			s.Peer(t).ExpectCmd(205, "QUIT")
			out := s.Dicmd(t, "dumpfeed", "peer")
			if want := "IncomingPriority: " + c.want + "\n"; !strings.Contains(out, want) {
				t.Errorf("incomingpriority %s: dicmd dumpfeed peer has no %q:\n%s", c.value, want, out)
			}
		})
	}
}

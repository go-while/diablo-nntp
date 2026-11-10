package e2e

import (
	"strings"
	"testing"
)

// TestDeepRevDumpfeedWithoutGlobal (L2-09): "dicmd dumpfeed" without a label prints every
// feed, starting with the GLOBAL label, and DumpAllFeedInfo() took that label's name through
// a pointer that is NULL when dnewsfeeds has no "label GLOBAL": the diablo master crashed.
// The command must print the other sections and the master must keep running.
func TestDeepRevDumpfeedWithoutGlobal(t *testing.T) {
	s := NewSite(t, SiteOptions{Newsfeeds: "label peer\n    inhost " + PeerIP + "\n    addgroup *\nend\n"})
	// a session first, so the master has loaded dnewsfeeds
	s.Peer(t).ExpectCmd(205, "QUIT")

	out, err := s.TryRun("dicmd", "dumpfeed")
	if err != nil || !strings.Contains(out, "** LABELS **") || !strings.Contains(out, "peer") {
		t.Errorf("dicmd dumpfeed without a GLOBAL label in dnewsfeeds: %v, output %q; want the LABELS section with the label peer", err, out)
	}
	select {
	case <-s.Proc.Done():
		t.Fatalf("the diablo master exited after dicmd dumpfeed: %s", s.logOutput())
	default:
	}
	s.Peer(t).ExpectCmd(205, "QUIT")
}

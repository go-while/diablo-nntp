package e2e

import (
	"strings"
	"testing"
	"time"
)

// TestDeepRevWhereisXrectimeMiss (L1-06): "whereis" and "xrectime" look the Message-ID up in
// the history and, on a miss, tested the expired flag of a History record that
// HistoryLookup() had not filled (it copies the record only on a hit). The reply for an
// unknown Message-ID then came from what the stack held: after a request for an expired
// article it was that article's record, so the unknown one was "430 Article expired" too.
// It must be "430 Article not found", as for HEAD and STAT.
func TestDeepRevWhereisXrectimeMiss(t *testing.T) {
	s := NewSite(t, SiteOptions{
		Newsfeeds: GlobalLabel + "label peer\n    inhost " + PeerIP + "\n    addgroup *\n    whereis yes\nend\n",
		SpoolCtl:  "spool 00\n    minfree 1m\n    keeptime 1\nend\nmetaspool trad\n    spool 00\nend\nexpire * trad\n",
	})
	// An article that dexpire has expired: its history record has the expired flag.
	old := s.Inject(t, "e2e.test", 1)[0]
	time.Sleep(2 * time.Second) // past keeptime 1 (seconds)
	newerSpoolDir(t, s)         // dexpire keeps a spool's newest directory
	s.Run(t, "dexpire", "-a")

	p := s.Peer(t)
	for _, cmd := range []string{"xrectime", "whereis"} {
		if code, line := p.Cmd("%s %s", cmd, old); code != 430 || !strings.Contains(line, "expired") {
			t.Fatalf("%s for the expired article: %q, want \"430 Article expired\"", cmd, line)
		}
		for range 3 {
			code, line := p.Cmd("%s %s", cmd, msgID(t))
			if code != 430 || !strings.Contains(line, "not found") {
				t.Errorf("%s for a Message-ID that is not in the history: %q, want \"430 Article not found\"", cmd, line)
			}
		}
	}
	p.ExpectCmd(205, "QUIT")
}

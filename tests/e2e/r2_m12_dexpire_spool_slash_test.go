package e2e

import (
	"testing"
)

// TestR2DexpireSpoolPathSlash (deep review r2 M12): with path_spool spelled with a trailing
// slash, dexpire's spool object names ("<path_spool>/" + "P.nn/" or the spool home itself)
// match no name the history scan builds, so dexpire -h1 marks every history entry of the
// spool expired and the feeder answers 430 for articles that are still stored. Nothing is
// old enough to expire here: both articles must still be there (STAT 223).
func TestR2DexpireSpoolPathSlash(t *testing.T) {
	s := NewSite(t, SiteOptions{Config: map[string]string{"path_spool": "@home@/spool/news/"}})
	ids := s.Inject(t, "e2e.test", 2)
	p := s.Peer(t)
	for _, id := range ids {
		p.ExpectCmd(223, "STAT %s", id)
	}
	p.ExpectCmd(205, "QUIT")

	out := s.Run(t, "dexpire", "-a", "-h1", "-v")

	p = s.Peer(t)
	for _, id := range ids {
		if code, line := p.Cmd("STAT %s", id); code != 223 {
			t.Errorf("STAT %s after dexpire -a -h1: %q, want 223 (nothing was old enough to expire)", id, line)
		}
	}
	p.ExpectCmd(205, "QUIT")
	if t.Failed() {
		t.Logf("dexpire output:\n%s", out)
	}
}

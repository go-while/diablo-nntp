package e2e

import (
	"strings"
	"testing"

	"github.com/go-while/nntp-testkit/client"
)

// TestSpoolCtlDontstoreWithoutSpools (D-107): a "dontstore yes" metaspool needs no spool lines.
// With allocstrat single, diablo divided by its number of spools (0) when it allocated the
// spools at start; with the others it indexed spool -1. Now such a metaspool is skipped
// there, diablo starts and stores articles of the other metaspools as before.
func TestSpoolCtlDontstoreWithoutSpools(t *testing.T) {
	s := NewSite(t, SiteOptions{SpoolCtl: `spool 00
    minfree 1m
end
metaspool trash
    dontstore yes
    allocstrat single
end
metaspool trad
    spool 00
end
expire junk.* trash
expire * trad
`})
	id := msgID(t)
	p := s.Peer(t)
	p.ExpectCmd(335, "IHAVE %s", id)
	p.MustSend(client.Stuff(newArticle(id, "e2e.test", "dontstore", "body")))
	p.ExpectStatus(235, "IHAVE "+id)
	p.ExpectCmd(223, "STAT %s", id)
	p.ExpectCmd(205, "QUIT")
}

// TestSpoolCtlTooManySpoolLines (D-107): a metaspool holds at most 100 spools; a 101st
// "spool" line is an error that stops the program. Before, it was written past the end of the
// metaspool's spool array.
func TestSpoolCtlTooManySpoolLines(t *testing.T) {
	ctl := "spool 00\n    minfree 1m\nend\nmetaspool trad\n" + strings.Repeat("    spool 00\n", 101) + "end\nexpire * trad\n"
	s := NewSite(t, SiteOptions{NoStart: true, SpoolCtl: ctl,
		Config: map[string]string{"path_generallog": "%s/general.log"}})
	out, err := s.TryRun("dexpire", "-n")
	if log := generalLog(s); err == nil || !strings.Contains(log, "More than 100 spool lines") {
		t.Errorf("dexpire -n with 101 spool lines in a metaspool: %v\n%s\ngeneral.log:\n%s", err, out, log)
	}
}

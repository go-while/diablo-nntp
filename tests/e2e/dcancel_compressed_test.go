package e2e

import (
	"strings"
	"testing"
)

// TestDcancelCompressedSpool: dcancel -a and -s rewrite a spool file of a compressed spool object
// (dspool.ctl compresslvl 6). The middle one of three articles is cancelled: the first is copied
// before it and the third after it, each as its stored gzip member with its terminator, and the
// history of both points at the new offsets. ARTICLE through diablo and dreadart still return
// the right articles, the cancelled one is gone. Before, dcancel stopped with "Compressed spools
// are currently not supported" and left every article in place; its code for compressed
// articles was compiled out (X_USE_ZLIB) and could not have built.
func TestDcancelCompressedSpool(t *testing.T) {
	skipWithoutZlib(t)
	for _, mode := range []string{"-a", "-s"} {
		t.Run(mode, func(t *testing.T) {
			s := NewSite(t, SiteOptions{SpoolCtl: compressedSpoolCtl})
			ids := s.Inject(t, "e2e.test", 3)
			out := dcancelRun(t, s, ids[1]+"\n", mode)
			if strings.Contains(out, "not supported") || !strings.Contains(out, "Skipped article") {
				t.Fatalf("dcancel %s on a compressed spool:\n%s", mode, out)
			}
			p := s.Peer(t)
			defer p.ExpectCmd(205, "QUIT")
			if code, line := p.Cmd("STAT %s", ids[1]); code == 223 {
				t.Errorf("the cancelled article is still there: %d %s", code, line)
			}
			for _, i := range []int{0, 2} {
				id := ids[i]
				p.ExpectCmd(220, "ARTICLE %s", id)
				art := strings.Join(p.ReadDotLines(), "\n")
				if !strings.Contains(art, "Message-ID: "+id) || !strings.Contains(art, "body of "+id) {
					t.Errorf("ARTICLE %s after dcancel %s returned:\n%s", id, mode, art)
				}
				da := s.Run(t, "dreadart", id)
				if !strings.Contains(da, "Message-ID: "+id) || !strings.Contains(da, "body of "+id) {
					t.Errorf("dreadart %s after dcancel %s:\n%s", id, mode, da)
				}
			}
		})
	}
}

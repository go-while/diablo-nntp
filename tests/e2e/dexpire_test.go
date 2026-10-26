package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestDexpire: dexpire -a removes the spool directory of an article older than the spool's
// keeptime; diablo then answers STAT for it with 430, and the history still remembers it, so
// the same article offered again is refused (435) instead of stored a second time.
func TestDexpire(t *testing.T) {
	s := NewSite(t, SiteOptions{SpoolCtl: `spool 00
    minfree 1m
    keeptime 1
end
metaspool trad
    spool 00
end
expire * trad
`})
	// The site's 127.0.0.2 label may also read from the spool (STAT).
	id := msgID(t)
	p := s.Peer(t)
	expect(t, p, 335, "IHAVE %s", id)
	send(t, p, wire(newArticle(id, "e2e.test", "to be expired", "old body")))
	status(t, p, 235, "IHAVE "+id)
	expect(t, p, 223, "STAT %s", id)
	expect(t, p, 205, "QUIT")

	dirs, _ := filepath.Glob(filepath.Join(s.Dir, "spool", "news", "D.*"))
	if len(dirs) == 0 {
		t.Fatalf("no spool directory after storing %s", id)
	}
	time.Sleep(2 * time.Second) // past keeptime 1 (seconds)
	out := s.Run(t, "dexpire", "-a", "-v")

	for _, d := range dirs {
		if _, err := os.Stat(d); err == nil {
			t.Errorf("spool directory %s still there after dexpire -a\ndexpire output:\n%s", d, out)
		}
	}
	p = s.Peer(t)
	expect(t, p, 430, "STAT %s", id)
	if code, line := p.Cmd("IHAVE %s", id); code != 435 {
		t.Errorf("IHAVE of the expired %s: %q, want 435 (still in history)", id, line)
		if code == 335 {
			send(t, p, wire(newArticle(id, "e2e.test", "to be expired", "old body")))
			_, _, _ = p.ReadStatus()
		}
	}
	expect(t, p, 205, "QUIT")
	if strings.Contains(out, "error") {
		t.Logf("dexpire output mentions an error:\n%s", out)
	}
}

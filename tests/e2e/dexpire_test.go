package e2e

import (
	"fmt"
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
	newerSpoolDir(t, s)         // dexpire keeps a spool's newest directory
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

// newerSpoolDir creates an empty spool directory for the next minute, as the next time slot
// gives a real site: dexpire never removes a spool's newest directory.
func newerSpoolDir(tb testing.TB, s *Site) {
	tb.Helper()
	d := filepath.Join(s.Dir, "spool", "news", fmt.Sprintf("D.%08x", time.Now().Unix()/60+1))
	if err := os.Mkdir(d, 0o755); err != nil {
		tb.Fatal(err)
	}
}

// TestDexpireKeepsNewestDir: dexpire -a doesn't remove a spool's newest directory, even when
// it is older than keeptime: diablo may be writing into it. The article in it stays
// readable, and the next article is stored.
func TestDexpireKeepsNewestDir(t *testing.T) {
	s := NewSite(t, SiteOptions{SpoolCtl: "spool 00\n    minfree 1m\n    keeptime 1\nend\nmetaspool trad\n    spool 00\nend\nexpire * trad\n"})
	id := s.Inject(t, "e2e.test", 1)[0]
	dirs, _ := filepath.Glob(filepath.Join(s.Dir, "spool", "news", "D.*"))
	if len(dirs) != 1 {
		t.Fatalf("spool directories: %q, want one", dirs)
	}
	time.Sleep(2 * time.Second)
	out := s.Run(t, "dexpire", "-a", "-v")
	if _, err := os.Stat(dirs[0]); err != nil {
		t.Fatalf("newest spool directory removed by dexpire -a: %v\n%s", err, out)
	}
	if !strings.Contains(out, "Keeping") {
		t.Errorf("dexpire didn't say it kept the newest directory:\n%s", out)
	}
	p := s.Peer(t)
	expect(t, p, 223, "STAT %s", id)
	next := msgID(t)
	expect(t, p, 335, "IHAVE %s", next)
	send(t, p, wire(newArticle(next, "e2e.test", "after dexpire", "body")))
	status(t, p, 235, "IHAVE after dexpire")
	expect(t, p, 223, "STAT %s", next)
	expect(t, p, 205, "QUIT")
}

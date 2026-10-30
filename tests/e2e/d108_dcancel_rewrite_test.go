package e2e

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// spoolFileOf returns the spool article file (B.xxxx) of the site that holds the article id.
func spoolFileOf(tb testing.TB, s *Site, id string) string {
	tb.Helper()
	var found string
	err := filepath.WalkDir(filepath.Join(s.Dir, "spool"), func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if found == "" && d.Type().IsRegular() && strings.HasPrefix(d.Name(), "B.") {
			if b, err := os.ReadFile(p); err == nil && bytes.Contains(b, []byte(id)) {
				found = p
			}
		}
		return nil
	})
	if err != nil || found == "" {
		tb.Fatalf("no spool file holds %s: %v", id, err)
	}
	return found
}

// D-108: dcancel stores the history offsets of the copied articles while it writes the rewritten
// spool file, before that file is renamed over the old one. If the rewrite then fails, history
// points into the old file at offsets of the rewritten one. Here the third article's spool header
// is corrupted, so the rewrite stops after the second article was copied to offset 0, where the
// cancelled first article is. The second article must still be read from its own data.
func TestDcancelFailedRewriteKeepsHistory(t *testing.T) {
	s := NewSite(t, SiteOptions{})
	ids := s.Inject(t, "e2e.test", 3)

	// Articles are ASCII, so the magic bytes of a spool header (0xff 0x99) occur only in headers.
	spool := spoolFileOf(t, s, ids[2])
	b, err := os.ReadFile(spool)
	if err != nil {
		t.Fatal(err)
	}
	at := bytes.Index(b, []byte(ids[2]))
	start := bytes.LastIndex(b[:at], []byte{0xff, 0x99})
	if start < 0 {
		t.Fatalf("no spool header before %s in %s", ids[2], spool)
	}
	b[start] = 0 // Magic1: dcancel stops with "Invalid article header"
	if err := os.WriteFile(spool, b, 0o644); err != nil {
		t.Fatal(err)
	}

	out := dcancelRun(t, s, ids[0]+"\n", "-a")
	t.Logf("dcancel -a:\n%s", out)
	if !strings.Contains(out, "Invalid article header") {
		t.Fatalf("the spool rewrite did not stop at the corrupt header:\n%s", out)
	}

	p := s.Peer(t)
	code, line := p.Cmd("STAT %s", ids[1])
	t.Logf("STAT 1 -> %d %s", code, line)
	if code != 223 {
		t.Errorf("article 1 (%s) after the failed rewrite: %d %s", ids[1], code, line)
	}
	code, line = p.Cmd("ARTICLE %s", ids[1])
	t.Logf("ARTICLE 1 -> %d %s", code, line)
	if code == 220 {
		body := p.ReadDotLines()
		if !strings.Contains(strings.Join(body, "\n"), ids[1]) {
			t.Errorf("ARTICLE %s after the failed rewrite returned a different article:\n%s",
				ids[1], strings.Join(body, "\n"))
		}
	} else {
		t.Errorf("ARTICLE %s after the failed rewrite: %d %s", ids[1], code, line)
	}
}

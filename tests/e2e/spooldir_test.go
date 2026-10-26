package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestSpoolDirRemovedWhileWriting: when the spool directory a connection is writing into is
// removed (as dexpire does when it expires down to the newest directory), the next article on
// the same connection is still stored: diablo recreates the directory and changes into it
// again. Before, it kept retrying inside the removed directory and rejected the article with
// "IOError/Missing-dir" (436).
func TestSpoolDirRemovedWhileWriting(t *testing.T) {
	s := NewSite(t, SiteOptions{})
	p := s.Peer(t)
	first := msgID(t)
	expect(t, p, 335, "IHAVE %s", first)
	send(t, p, wire(newArticle(first, "e2e.test", "first", "body")))
	status(t, p, 235, "IHAVE first")

	dirs, _ := filepath.Glob(filepath.Join(s.Dir, "spool", "news", "D.*"))
	if len(dirs) == 0 {
		t.Fatal("no spool directory after the first article")
	}
	for _, d := range dirs {
		if err := os.RemoveAll(d); err != nil {
			t.Fatal(err)
		}
	}

	second := msgID(t)
	expect(t, p, 335, "IHAVE %s", second)
	send(t, p, wire(newArticle(second, "e2e.test", "second", "body")))
	status(t, p, 235, "IHAVE after the spool directory was removed")
	expect(t, p, 223, "STAT %s", second)
	expect(t, p, 205, "QUIT")

	if again, _ := filepath.Glob(filepath.Join(s.Dir, "spool", "news", "D.*")); len(again) == 0 {
		t.Errorf("spool directory not recreated")
	}
	if e := s.Proc.Stderr(); !strings.Contains(e, "was missing, created it") && !strings.Contains(e, "was removed while open") {
		t.Errorf("no log line about the recreated spool directory")
	}
}

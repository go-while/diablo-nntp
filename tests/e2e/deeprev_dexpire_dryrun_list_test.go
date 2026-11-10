package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestDeepRevDexpireDryRunList (L7-12): "dexpire -o" appends the hash of every history record
// it expires to the dexpover list, which "dexpireover -o" reads to expire the same articles
// from the overview. With -n (the default without -a) dexpire changes nothing, but it still
// opened the list and wrote the hashes of the records it would have expired, so a dry run
// left a list that the next dexpireover -o acted on.
//
// A spool with keeptime 1 s and one article in a directory that is due: dexpire -n -o must
// not create the list, dexpire -a -o writes one hash (8 bytes) to it.
func TestDeepRevDexpireDryRunList(t *testing.T) {
	s := NewSite(t, SiteOptions{SpoolCtl: "spool 00\n    minfree 1m\n    keeptime 1\nend\nmetaspool trad\n    spool 00\nend\nexpire * trad\n"})
	s.Inject(t, "e2e.test", 1)
	dirs, _ := filepath.Glob(filepath.Join(s.Dir, "spool", "news", "D.*"))
	if len(dirs) != 1 {
		t.Fatalf("spool directories: %q, want one", dirs)
	}
	time.Sleep(2 * time.Second) // past keeptime 1 (seconds)
	newerSpoolDir(t, s)
	list := filepath.Join(strings.TrimSpace(s.Run(t, "dpath", "db")), "dexpover.dat")
	if _, err := os.Stat(list); err == nil {
		t.Fatalf("the list %s exists before dexpire ran", list)
	}

	out, err := s.TryRun("dexpire", "-n", "-o", "-v")
	if err != nil {
		t.Fatalf("dexpire -n -o -v: %v\n%s", err, out)
	}
	if st, err := os.Stat(list); err == nil {
		t.Errorf("dexpire -n -o wrote the list %s (%d bytes)\n%s", list, st.Size(), out)
		os.Remove(list)
	}
	if _, err := os.Stat(dirs[0]); err != nil {
		t.Fatalf("dexpire -n removed the spool directory: %v\n%s", err, out)
	}

	out, err = s.TryRun("dexpire", "-a", "-o", "-v")
	if err != nil {
		t.Fatalf("dexpire -a -o -v: %v\n%s", err, out)
	}
	if st, err := os.Stat(list); err != nil || st.Size() != 8 {
		t.Errorf("dexpire -a -o: list %s: %v, want one hash of 8 bytes\n%s", list, err, out)
	}
}

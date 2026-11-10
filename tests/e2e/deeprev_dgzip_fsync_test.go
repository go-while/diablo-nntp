package e2e

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

// TestDeepRevDgzipSyncBeforeRemove (L2-11): dexpireover -z compresses an old overview data file
// with diablo_minigzip() (lib/dgzip.c), which wrote the .gz, renamed it in and removed the
// original with no fsync(): after a crash the name could hold an empty or short .gz and the
// original was gone. The copy is now synced first, and then the directory with its new name
// (the unlink could otherwise reach the disk before the rename); when either sync fails the
// original stays and no .gz is left.
//
// A crash cannot be made here. A preloaded fsync() fails for the files under the group
// directory (subtest file), or only for the directory (subtest dir), instead: without the
// sync dexpireover never sees that and compresses; with it both data files stay plain, and
// dreaderd still serves every article.
func TestDeepRevDgzipSyncBeforeRemove(t *testing.T) {
	skipWithoutZlib(t)
	so := preloadShim(t, "fsyncfail", fsyncFailSource)
	for _, dirOnly := range []bool{false, true} {
		name := "file"
		if dirOnly {
			name = "dir"
		}
		t.Run(name, func(t *testing.T) { deepRevDgzipSync(t, so, dirOnly) })
	}
}

func deepRevDgzipSync(t *testing.T, so string, dirOnly bool) {
	mark := filepath.Join(t.TempDir(), "fsynced")
	st, spool := spoolStore(t)
	// e4 and 12 articles: four data files; F1 keeps the two newest plain, Z1 lets the two
	// older ones (made two hours old) be compressed (as in TestReaderReadsGzipData)
	s := NewReaderSite(t, ReaderOptions{Spool: spool,
		Files: map[string]string{"dexpire.ctl": "e2e.*:x30:e4:F1:Z1\n"}})
	var arts [][]string
	for n := 1; n <= 12; n++ {
		a := newArticle(msgID(t), "e2e.test", fmt.Sprintf("gzip sync %02d", n), "body")
		head, body := splitArticle(a)
		arts = append(arts, append(append(head, ""), body...))
	}
	feedReader(t, s, st, "e2e.test", arts...)

	group := filepath.Join(s.Dir, "spool", "group")
	dataFiles := func() (plain, gz []string) {
		m, _ := filepath.Glob(filepath.Join(group, "*", "data.*"))
		m2, _ := filepath.Glob(filepath.Join(group, "data.*"))
		for _, f := range append(m, m2...) {
			if strings.HasSuffix(f, ".gz") {
				gz = append(gz, f)
			} else {
				plain = append(plain, f)
			}
		}
		return plain, gz
	}
	plain, _ := dataFiles()
	if len(plain) != 4 {
		t.Fatalf("%d data files, want 4 (e4, 12 articles): %q", len(plain), plain)
	}
	old := time.Now().Add(-2 * time.Hour)
	for _, f := range plain {
		if err := os.Chtimes(f, old, old); err != nil {
			t.Fatal(err)
		}
	}

	base := s.Env
	s.Env = append(slices.Clone(base), lockFaultEnv(so), "FSYNCFAIL_FILE="+group+"/", "FSYNCFAIL_MARK="+mark)
	if dirOnly {
		// the directories of the data files are below group
		s.Env = append(slices.Clone(base), lockFaultEnv(so), "FSYNCFAIL_FILE="+group, "FSYNCFAIL_DIRONLY=1", "FSYNCFAIL_MARK="+mark)
	}
	out, err := s.TryRun("dexpireover", "-y", "-z", "-v")
	s.Env = base
	t.Logf("dexpireover -y -z -v: err=%v\n%s", err, out)

	if b, err := os.ReadFile(mark); err != nil || len(b) == 0 {
		t.Errorf("dexpireover did not sync the compressed copy (dir only: %v): the fault was never hit (%v)", dirOnly, err)
	}
	plain, gz := dataFiles()
	if len(gz) != 0 || len(plain) != 4 {
		t.Errorf("after a failed sync: %d .gz and %d plain data files, want 0 and 4 (the originals kept):\n%q\n%q",
			len(gz), len(plain), gz, plain)
	}
	if n := strings.Count(out, "Unable to compress"); n != 2 {
		t.Errorf("dexpireover reported %d \"Unable to compress\" lines, want 2", n)
	}
	if left, _ := filepath.Glob(filepath.Join(group, "*", ".data.*")); len(left) != 0 {
		t.Errorf("temporary .gz files left behind: %q", left)
	}

	c := s.Reader(t)
	c.ExpectCmd(211, "GROUP e2e.test")
	c.ExpectCmd(224, "OVER 1-12")
	if n := len(c.ReadDotLines()); n != 12 {
		t.Errorf("OVER 1-12 after the failed compression: %d lines, want 12", n)
	}
	c.ExpectCmd(205, "QUIT")
}

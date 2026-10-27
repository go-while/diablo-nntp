package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	testkit "github.com/go-while/nntp-testkit"
)

// overviewSize feeds one article for e2e.test to a reader site whose dexpire.ctl is ctl and
// returns the size of the group's overview index file, which dreaderd creates with room for
// the "a" (initial articles) value of the matching dexpire.ctl rule.
func overviewSize(t *testing.T, ctl string) int64 {
	t.Helper()
	st, spool := spoolStore(t)
	s := NewReaderSite(t, ReaderOptions{Spool: spool, Files: map[string]string{"dexpire.ctl": ctl}})
	feedReader(t, s, st, "e2e.test", newArticle(msgID(t), "e2e.test", "overview size", "body"))
	c := s.Reader(t)
	waitFor(t, testkit.Deadline(), "the article in GROUP e2e.test", func() bool {
		_, line := c.Cmd("GROUP e2e.test")
		return strings.HasPrefix(line, "211 1")
	})
	expect(t, c, 205, "QUIT")
	var size int64 = -1
	filepath.WalkDir(filepath.Join(s.Dir, "spool", "group"), func(p string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() && strings.HasPrefix(d.Name(), "over.") {
			if fi, err := d.Info(); err == nil {
				size = fi.Size()
			}
		}
		return nil
	})
	if size < 0 {
		t.Fatalf("no overview file under %s/spool/group", s.Dir)
	}
	return size
}

// TestDexpireCtlIndent (D-83): loadExpireCtl() skips the white space in front of a line but
// then copies the pattern from the start of the buffer instead of from the first non-blank,
// so an indented rule has a wildcard with leading blanks that matches no group and the rule
// is silently ignored.
func TestDexpireCtlIndent(t *testing.T) {
	plain := overviewSize(t, "e2e.*:a4000\n")
	def := overviewSize(t, "other.*:a4000\n") // a rule that doesn't match: the default size
	if plain == def {
		t.Fatalf("the rule e2e.*:a4000 doesn't change the overview size (%d); test setup is wrong", plain)
	}
	if got := overviewSize(t, "   e2e.*:a4000\n"); got != plain {
		t.Errorf("indented dexpire.ctl rule \"   e2e.*:a4000\" is ignored: overview file is %d bytes, %d bytes with the same rule not indented (default %d)", got, plain, def)
	}
}

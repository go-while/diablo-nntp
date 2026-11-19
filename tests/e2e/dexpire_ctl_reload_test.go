package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestReaderDexpireCtlMissingAtReload (D-270): a reader process re-reads dexpire.ctl when its
// minute changes. A file that cannot be opened at that moment (briefly missing while an admin
// replaces it) stopped the reader process, and with it every client connection it held. The
// process must keep running with the list it loaded and read the file again at a later check.
// The reader checks about every 30 to 50 seconds (the first check later), and only a new
// minute opens the file, so the test waits 110 seconds for one such check; on main the
// connection is closed within that time. The log line of the failed open must name
// dexpire.ctl (it once named the log file: the path buffer was reused by the log call).
func TestReaderDexpireCtlMissingAtReload(t *testing.T) {
	_, spool := spoolStore(t)
	s := NewReaderSite(t, ReaderOptions{Spool: spool})
	c := s.Reader(t)
	c.ExpectCmd(111, "DATE")

	if err := os.Remove(filepath.Join(s.Dir, "dexpire.ctl")); err != nil {
		t.Fatal(err)
	}
	if lines, err := c.ExpectClosed(110 * time.Second); err == nil {
		t.Fatalf("the reader closed its client connection (lines %q) after dexpire.ctl was missing at a reload: the reader process stopped", lines)
	}
	c.ExpectCmd(111, "DATE")
	c.ExpectCmd(205, "QUIT")
	if g := generalLog(s); !strings.Contains(g, "dexpire.ctl cannot be opened at reload") {
		t.Errorf("the general log has no line naming dexpire.ctl as not openable at the reload:\n%s", g)
	}
}

package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestDeepRevDhisctlLastLine (L7-18): dhisctl -f reads Message-IDs from a file and removed
// the last character of each line as the newline, whatever it was. The last line of a file
// that does not end with a newline lost its ">" and was refused as an invalid Message-ID
// (exit 1); an empty line (a NUL as its first byte) wrote one byte before the buffer.
// dhisctl now cuts the line at the newline.
func TestDeepRevDhisctlLastLine(t *testing.T) {
	s := NewSite(t, SiteOptions{})
	id := s.Inject(t, "e2e.test", 1)[0]
	list := filepath.Join(t.TempDir(), "ids")
	if err := os.WriteFile(list, []byte(id), 0o644); err != nil { // no newline
		t.Fatal(err)
	}
	out, err := s.TryRun("dhisctl", "-e", "-v", "-f", list, filepath.Join(s.Dir, "dhistory"))
	if err != nil {
		t.Errorf("dhisctl -f with a last line without a newline: %v\n%s", err, out)
	}
	if !strings.Contains(out, "Expiring: "+id) {
		t.Errorf("dhisctl -e -v did not expire %s:\n%s", id, out)
	}
}

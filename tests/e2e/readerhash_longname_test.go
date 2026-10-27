package e2e

import (
	"strings"
	"testing"
)

// TestReaderHashHierLongGroupName (GHSA-5cwh-wc3c-f5w5): with "readergrouphash hierarchy" the
// overview path of a group is about twice as long as its name; dreaderd allocated the path
// buffer as the group home plus 48 bytes. An 88-character group name must open its overview
// and answer GROUP and LISTGROUP without corrupting the heap.
func TestReaderHashHierLongGroupName(t *testing.T) {
	name := "e2e." + strings.Repeat("longname.", 9) + "end"
	_, spool := spoolStore(t)
	s := NewReaderSite(t, ReaderOptions{
		Spool:  spool,
		Groups: []string{name},
		Config: map[string]string{"readergrouphash": "hierarchy"},
	})
	p := s.Reader(t)
	for i := 0; i < 3; i++ {
		code, line := p.Cmd("GROUP %s", name)
		if code != 211 {
			t.Fatalf("GROUP %s (round %d): %d %s", name, i, code, line)
		}
		// LISTGROUP opens the group's overview file (GetOverInfo)
		code, line = p.Cmd("LISTGROUP %s", name)
		if code != 211 {
			t.Fatalf("LISTGROUP (round %d): %d %s", i, code, line)
		}
		p.ReadDotLines()
	}
}

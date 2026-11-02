package e2e

import (
	"os"
	"os/exec"
	"path/filepath"
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

// TestDgrpctlMasscancelHierLongGroupName: dgrpctl's own copy of GetOverInfo() still allocated
// the overview path as the group home plus 48 bytes (the dreaderd copy was fixed for
// GHSA-5cwh-wc3c-f5w5). With "readergrouphash hierarchy" and an 88-character group name,
// "dgrpctl masscancel" wrote 131 bytes past it; now it opens the group's overview
// and reads the Message-IDs.  The buffer comes from Diablo's pool allocator, which ASan does
// not see into, so the old code passes this test too: it guards the new path code.
func TestDgrpctlMasscancelHierLongGroupName(t *testing.T) {
	name := "e2e." + strings.Repeat("longname.", 9) + "end"
	s := NewReaderSite(t, ReaderOptions{
		SpoolAddr: "127.0.0.1:1",
		Groups:    []string{name},
		Config:    map[string]string{"readergrouphash": "hierarchy"},
		NoStart:   true,
	})
	cmd := exec.Command(filepath.Join(s.Bin, "dgrpctl"), "masscancel", name)
	cmd.Dir = s.Dir
	cmd.Env = append(os.Environ(), s.Env...)
	cmd.Stdin = strings.NewReader("bogus\n")
	b, err := cmd.CombinedOutput()
	out := string(b)
	noSanitizerReport(t, out)
	if err != nil {
		t.Fatalf("dgrpctl masscancel: %v\n%.600s", err, out)
	}
	if !strings.Contains(out, "invalid message-id: bogus") || strings.Contains(out, "invalid group") {
		t.Errorf("dgrpctl masscancel output:\n%.600s", out)
	}
}

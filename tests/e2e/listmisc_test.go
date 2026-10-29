package e2e

import (
	"strings"
	"testing"
)

// TestReaderListModeratorsCRLF: LIST MODERATORS on a moderators file with CRLF line ends
// sends each line once with CRLF, not with the file's CR in front of it (CR CR LF). Before,
// only the LF was stripped.
func TestReaderListModeratorsCRLF(t *testing.T) {
	m := newModerationReader(t, ModOptions{Moderators: "e2e.mod:a@moderators.example.invalid\r\ne2e.mod2:b@moderators.example.invalid\r\n"})
	c := m.Reader(t)
	expect(t, c, 215, "LIST MODERATORS")
	lines := c.ReadDotLines()
	if len(lines) != 2 {
		t.Fatalf("LIST MODERATORS: %q, want 2 lines", lines)
	}
	for _, l := range lines {
		if strings.ContainsAny(l, "\r\n") {
			t.Errorf("LIST MODERATORS line %q holds a stray CR", l)
		}
	}
}

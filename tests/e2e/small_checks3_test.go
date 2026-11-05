package e2e

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestSpoolCtlUnknownOption (D-120): an unknown option inside a spool or metaspool block of
// dspool.ctl (here a typo of minfree) stops the program, as an unknown definition already
// does. Before, it was only logged and the spool ran with the default, without a free-space
// floor.
func TestSpoolCtlUnknownOption(t *testing.T) {
	for _, tc := range []struct{ name, ctl string }{
		{"spool", "spool 00\n    minfre 1m\nend\nmetaspool trad\n    spool 00\nend\nexpire * trad\n"},
		{"metaspool", "spool 00\n    minfree 1m\nend\nmetaspool trad\n    spool 00\n    allocstart single\nend\nexpire * trad\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := NewSite(t, SiteOptions{NoStart: true, SpoolCtl: tc.ctl,
				Config: map[string]string{"path_generallog": "%s/general.log"}})
			out, err := s.TryRun("dexpire", "-n")
			log := generalLog(s)
			// on main dexpire went on and failed later, at the history this site lacks
			if err == nil || strings.Contains(out, "Unable to open history") || !strings.Contains(log, "Unknown "+tc.name+" option") {
				t.Errorf("dexpire -n with an unknown %s option: %v\n%s\ngeneral.log:\n%s", tc.name, err, out, log)
			}
		})
	}
}

// TestDicmdLongLine (D-127): a command line on dicmd's standard input longer than its 256-byte
// buffer is sent whole before dicmd reads the reply. Before, dicmd sent the first 255 bytes
// and waited for a reply that diablo, still waiting for the end of the line, never sent.
func TestDicmdLongLine(t *testing.T) {
	s := NewSite(t, SiteOptions{})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, filepath.Join(s.Bin, "dicmd"))
	cmd.Dir = s.Dir
	cmd.Env = append(os.Environ(), s.Env...)
	cmd.Stdin = strings.NewReader("help " + strings.Repeat("x", 600) + "\nversion\n")
	out, err := cmd.CombinedOutput()
	if ctx.Err() != nil {
		t.Fatalf("dicmd hung on a 605-byte command line\n%s", out)
	}
	if err != nil || strings.Count(string(out), "\n.\n")+boolInt(strings.HasPrefix(string(out), ".\n")) < 2 {
		t.Errorf("dicmd with a long line, then version: %v\n%s", err, out)
	}
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// TestReaderListLongFileLines (D-94): LIST MODERATORS and LIST DISTRIBUTIONS send a line of the
// local file that is longer than the 256-byte read buffer as one line. Before, each 255-byte
// piece went out as a line of its own, and the tail of the long line looked like an entry.
func TestReaderListLongFileLines(t *testing.T) {
	long := "e2e.long." + strings.Repeat("x", 400) + ":long@moderators.example.invalid"
	m := newModerationReader(t, ModOptions{Moderators: "# a comment\n" + long + "\ne2e.mod:a@moderators.example.invalid\n"})
	c := m.Reader(t)
	c.ExpectCmd(215, "LIST MODERATORS")
	lines := c.ReadDotLines()
	if len(lines) != 2 || lines[0] != long {
		t.Errorf("LIST MODERATORS: %d lines %.80q..., want the long line whole and one more", len(lines), lines)
	}

	dist := "local." + strings.Repeat("y", 300) + "\tA local distribution"
	if err := os.WriteFile(filepath.Join(m.Dir, "distributions"), []byte(dist+"\nworld\tEverywhere\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	c.ExpectCmd(215, "LIST DISTRIBUTIONS")
	lines = c.ReadDotLines()
	if len(lines) != 2 || lines[0] != dist {
		t.Errorf("LIST DISTRIBUTIONS: %d lines, want the long line whole and one more", len(lines))
	}
	c.ExpectCmd(205, "QUIT")
}

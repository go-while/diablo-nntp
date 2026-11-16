package e2e

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	testkit "github.com/go-while/nntp-testkit"
)

// TestMasscancelHighNumbers (D-220): the overview slot keeps 31 bits of the article number
// (oa_ArtNo), and dgrpctl masscancel compared it with the full number, so in a group whose
// numbers are above 2^31 - 1 no slot matched and nothing was cancelled. The article fed after
// NE=3000000000 gets a number above that; masscancel must cancel it and the reader then
// answers 423 for it.
func TestMasscancelHighNumbers(t *testing.T) {
	st, spool := spoolStore(t)
	s := NewReaderSite(t, ReaderOptions{Spool: spool})
	s.Run(t, "dgrpctl", "adjust", "e2e.test", "NE", "=3000000000")
	id := msgID(t)
	feedReader(t, s, st, "e2e.test", newArticle(id, "e2e.test", "high number", "body"))

	c := s.Reader(t)
	var high string
	testkit.WaitFor(t, 10*time.Second, "the article in GROUP", func() bool {
		_, line := c.Cmd("GROUP e2e.test")
		f := strings.Fields(line)
		if len(f) < 4 || f[0] != "211" || f[1] == "0" {
			return false
		}
		high = f[3]
		return true
	})
	if len(high) < 10 || high < "2147483648" {
		t.Fatalf("setup: high mark %s, want a number above 2^31 - 1", high)
	}
	c.ExpectCmd(223, "STAT %s", high)
	c.ExpectCmd(205, "QUIT")

	cmd := exec.Command(filepath.Join(s.Bin, "dgrpctl"), "masscancel", "e2e.test")
	cmd.Dir = s.Dir
	cmd.Env = append(os.Environ(), s.Env...)
	cmd.Stdin = strings.NewReader(id + "\n")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("dgrpctl masscancel: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), ">> cancelled e2e.test:"+high) {
		t.Errorf("masscancel did not cancel article %s (%s); output:\n%s", high, id, out)
	}

	c2 := s.Reader(t)
	c2.ExpectCmd(211, "GROUP e2e.test")
	c2.ExpectCmd(423, "STAT %s", high)
	c2.ExpectCmd(205, "QUIT")
}

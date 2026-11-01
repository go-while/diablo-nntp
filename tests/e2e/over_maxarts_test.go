package e2e

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// An over file that is only a header (no article slots) makes maxarts, computed as
// (filesize - headsize) / sizeof(OverArt), come out 0. dexpireover already guards this
// (TestDexpireoverHeaderOnlyOver); these cover the other paths that compute maxarts the
// same way and then take a modulo by it: dgrpctl groupinfo's own over-file dump, and the
// GetOverInfo() both dgrpctl (masscancel) and the reader daemon use.

// TestGroupinfoHeaderOnlyOver: dgrpctl groupinfo on a header-only over file reports the
// damaged file instead of dividing by zero and dying with SIGFPE.
func TestGroupinfoHeaderOnlyOver(t *testing.T) {
	s, home := overSite(t, map[string]string{"readergrouphash": "hierarchy"})
	writeOver(t, filepath.Join(home, "e2e", "o.0.e2e.test"), overHead("e2e.test"))

	out, err := s.TryRun("dgrpctl", "groupinfo", "e2e.test")
	var ee *exec.ExitError
	if errors.As(err, &ee) && !ee.Exited() {
		t.Errorf("dgrpctl groupinfo on a header-only over file was killed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "no article slots") {
		t.Errorf("the damaged over file was not reported:\n%s", out)
	}
}

// TestMasscancelHeaderOnlyOver: dgrpctl masscancel calls GetOverInfo() + GetOverArt() on the
// group; on a header-only over file GetOverInfo used to compute ov_MaxArts 0 and the first
// GetOverArt took a modulo by it. It now re-initialises the degenerate file (it holds no
// articles anyway) and runs to completion. Empty stdin = no message-ids to cancel.
func TestMasscancelHeaderOnlyOver(t *testing.T) {
	s, home := overSite(t, map[string]string{"readergrouphash": "hierarchy"})
	writeOver(t, filepath.Join(home, "e2e", "o.0.e2e.test"), overHead("e2e.test"))

	cmd := exec.Command(filepath.Join(s.Bin, "dgrpctl"), "masscancel", "e2e.test")
	cmd.Dir = s.Dir
	cmd.Env = append(os.Environ(), s.Env...)
	cmd.Stdin = strings.NewReader("")
	out, err := cmd.CombinedOutput()
	var ee *exec.ExitError
	if errors.As(err, &ee) && !ee.Exited() {
		t.Errorf("dgrpctl masscancel on a header-only over file was killed: %v\n%s", err, out)
	}
	// The degenerate file was repaired, not merely survived (returning NULL would
	// also pass the check above): groupinfo now reads a usable maxarts
	// (DEFARTSINGROUP = 512) instead of 0.
	info, _ := s.TryRun("dgrpctl", "groupinfo", "e2e.test")
	if !strings.Contains(info, "maxarts=512") {
		t.Errorf("over file not repaired to a usable size after masscancel:\n%s", info)
	}
}

// TestDcancelHeaderOnlyOver: dcancel -r reads group:number lines from stdin and cancels in
// the reader header index via its own GetOverInfo() + GetOverArt(). On a header-only over
// file it used to take a modulo by an ov_MaxArts of 0; it now re-initialises the file.
func TestDcancelHeaderOnlyOver(t *testing.T) {
	s, home := overSite(t, map[string]string{"readergrouphash": "hierarchy"})
	writeOver(t, filepath.Join(home, "e2e", "o.0.e2e.test"), overHead("e2e.test"))

	cmd := exec.Command(filepath.Join(s.Bin, "dcancel"), "-r")
	cmd.Dir = s.Dir
	cmd.Env = append(os.Environ(), s.Env...)
	cmd.Stdin = strings.NewReader("e2e.test:1\n")
	out, err := cmd.CombinedOutput()
	var ee *exec.ExitError
	if errors.As(err, &ee) && !ee.Exited() {
		t.Errorf("dcancel -r on a header-only over file was killed: %v\n%s", err, out)
	}
}

// TestDreadoverHeaderOnlyOver: dreadover group:artno computes maxarts the same way and takes
// a modulo by it; on a header-only over file it reports the damaged file instead of SIGFPE.
func TestDreadoverHeaderOnlyOver(t *testing.T) {
	s, home := overSite(t, map[string]string{"readergrouphash": "hierarchy"})
	writeOver(t, filepath.Join(home, "e2e", "o.0.e2e.test"), overHead("e2e.test"))

	out, err := s.TryRun("dreadover", "e2e.test:1")
	var ee *exec.ExitError
	if errors.As(err, &ee) && !ee.Exited() {
		t.Errorf("dreadover on a header-only over file was killed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "no article slots") {
		t.Errorf("the damaged over file was not reported:\n%s", out)
	}
}

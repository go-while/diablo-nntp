package e2e

import (
	"errors"
	"os/exec"
	"strings"
	"testing"
)

// TestDeepRevDexpireoverParallelCount (L7-17): "dexpireover -pN" starts N processes that
// share the group directories by (index of the first character) % N. The count had an upper
// limit only. With a negative N no process was started, and the one process went on with the
// directories whose index is a multiple of N: -p-3 looked at a third of the overview and
// reported a normal run. A negative count is now refused.
func TestDeepRevDexpireoverParallelCount(t *testing.T) {
	s, _ := overSite(t, nil)
	s.WriteFile(t, "dexpire.ctl", "e2e.*:x30\n")
	out, err := s.TryRun("dexpireover", "-n", "-y", "-p-3")
	var ee *exec.ExitError
	if !errors.As(err, &ee) || ee.ExitCode() != 1 {
		t.Errorf("dexpireover -p-3: %v, want exit status 1\n%s", err, out)
	}
	if !strings.Contains(out, "Invalid number of processes") {
		t.Errorf("dexpireover -p-3 did not refuse the count:\n%s", out)
	}
	if strings.Contains(out, "Scanned ") {
		t.Errorf("dexpireover -p-3 scanned the overview:\n%s", out)
	}
	// 0 is "not parallel", as without -p
	if out, err := s.TryRun("dexpireover", "-n", "-y", "-p0"); err != nil || !strings.Contains(out, "Scanned ") {
		t.Errorf("dexpireover -p0: %v\n%s", err, out)
	}
}

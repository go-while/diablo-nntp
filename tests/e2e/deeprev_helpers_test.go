package e2e

// Helpers shared by the deeprev_*_test.go files: tests for the findings of the deep review of
// main b37c724 (2026-11-09). Each of those tests asserts the correct behaviour, so it fails on
// b37c724 and passes once the fix for its finding is in.

import (
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"
)

// deepRevKillChildren registers a cleanup that sends SIGKILL to every process called name
// ("diablo", "dreaderd") of the site other than the daemon the harness started. A feeder
// session that loops (L1-01) ignores "dicmd exit" and SIGTERM, so without this the site's
// shutdown waits for it and the process outlives the test. Call it right after the site is
// started: it then runs before the site's own shutdown.
func deepRevKillChildren(t *testing.T, s *Site, name string) {
	t.Helper()
	t.Cleanup(func() {
		pids, err := trySitePids(s, name)
		if err != nil {
			t.Logf("deeprev cleanup: cannot list the %s processes of the site: %v", name, err)
			return
		}
		master := 0
		if s.Proc != nil {
			master = s.Proc.Pid()
		}
		for _, pid := range pids {
			if pid != master {
				_ = syscall.Kill(pid, syscall.SIGKILL)
			}
		}
	})
}

// deepRevWaitQueue flushes diablo's outgoing queues until dqueue/label has at least n lines
// or d has passed, and returns the lines it saw last. Unlike waitQueue it does not fail the
// test: the caller says what is missing.
func deepRevWaitQueue(t *testing.T, s *Site, label string, n int, d time.Duration) []string {
	t.Helper()
	limit := time.Now().Add(d)
	for {
		q := s.queue(t, label)
		if len(q) >= n || time.Now().After(limit) {
			return q
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// deepRevMissing returns the ids that no line of the queue q holds as a field of its own.
func deepRevMissing(q []string, ids []string) []string {
	var missing []string
	for _, id := range ids {
		if !slices.ContainsFunc(q, func(l string) bool { return slices.Contains(strings.Fields(l), id) }) {
			missing = append(missing, id)
		}
	}
	return missing
}

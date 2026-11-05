package e2e

import (
	"testing"
	"time"
)

// TestFeederExitWithoutCleanupIsQuick: without path_shutdown_cleanup, "dicmd exit" stops
// diablo without a fixed pause; with a cleanup program diablo still waits 2 s before it
// runs, as before. Fails on main: diablo always slept 2 s at the end of its shutdown, so
// every stop and restart (and every feeder test's cleanup) took 2 s longer than it needed.
func TestFeederExitWithoutCleanupIsQuick(t *testing.T) {
	s := NewSite(t, SiteOptions{})
	start := time.Now()
	if out, err := s.run("dicmd", "exit"); err != nil {
		t.Fatalf("dicmd exit: %v\n%s", err, out)
	}
	select {
	case <-s.Proc.Done():
	case <-time.After(10 * time.Second):
		t.Fatalf("diablo still running 10 s after dicmd exit")
	}
	// the old fixed pause alone was 2 s; a stop without it takes milliseconds
	if d := time.Since(start); d >= 1500*time.Millisecond {
		t.Errorf("diablo took %v to stop after dicmd exit, want well under the old 2 s pause", d)
	}
}

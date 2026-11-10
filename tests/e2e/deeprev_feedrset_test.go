package e2e

import (
	"syscall"
	"testing"
)

// TestDeepRevFeedrsetRepeated (L1-11): "feedrset" opens feeds/<label>.new for the feedadd and
// feeddel commands that follow. A second feedrset without a feedcommit opened the file again
// and forgot the first FILE, so each one cost the session a descriptor. When the open then
// failed, the session still took the table as ready from the feedrset before, and the next
// feedadd or feedcommit used the NULL FILE: the session process crashed.
//
// diablo runs with a limit of 256 descriptors here; 300 feedrset must all be answered 290,
// and feedadd and feedcommit must still work after them.
func TestDeepRevFeedrsetRepeated(t *testing.T) {
	var old syscall.Rlimit
	if err := syscall.Getrlimit(syscall.RLIMIT_NOFILE, &old); err != nil {
		t.Skipf("getrlimit: %v", err)
	}
	low := old
	low.Cur = 256
	if err := syscall.Setrlimit(syscall.RLIMIT_NOFILE, &low); err != nil {
		t.Skipf("setrlimit: %v", err)
	}
	restore := func() { _ = syscall.Setrlimit(syscall.RLIMIT_NOFILE, &old) }
	t.Cleanup(restore)
	s := NewSite(t, SiteOptions{}) // diablo inherits the limit
	restore()

	p := s.Peer(t)
	for i := 1; i <= 300; i++ {
		code, line, err := p.TryCmd("feedrset")
		if err != nil || code != 290 {
			t.Fatalf("feedrset number %d: %q, %v; want 290 (each feedrset before it left its file open)", i, line, err)
		}
	}
	if code, line, err := p.TryCmd("feedadd e2e.*"); err != nil || code != 290 {
		t.Fatalf("feedadd after 300 feedrset: %q, %v; want 290", line, err)
	}
	if code, line, err := p.TryCmd("feedcommit"); err != nil || code != 290 {
		t.Fatalf("feedcommit after 300 feedrset: %q, %v; want 290", line, err)
	}
	if got := s.ReadFile(t, "feeds/peer"); got != "addgroup\te2e.*\n" {
		t.Errorf("feeds/peer after feedrset, feedadd e2e.*, feedcommit: %q", got)
	}
	p.ExpectCmd(205, "QUIT")
}

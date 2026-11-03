package e2e

import (
	"fmt"
	"os"
	"strings"
	"syscall"
	"testing"
)

// TestFeederOutqRunning: OUTQ from a peer reports the backlog of its label's outgoing queue
// (dqueue/.peer.seq names the queue files) and how many of the files a dnewslink is working on,
// which it sees by the write lock dnewslink holds on all of a queue file. The test holds such a
// lock on one of three files. Before the fix the probe tried to take that lock itself on a
// descriptor opened read-only, which always fails (EBADF), so every queue file counted as
// running: now-running=3 with no dnewslink at all.
func TestFeederOutqRunning(t *testing.T) {
	s := NewSite(t, SiteOptions{})
	s.WriteFile(t, "dqueue/.peer.seq", "0 3\n")
	for i := 0; i < 3; i++ {
		s.WriteFile(t, fmt.Sprintf("dqueue/peer.S%05d", i),
			fmt.Sprintf("D.00000001/B.0001 <q%d-1@e2e.test> 0,100\nD.00000001/B.0001 <q%d-2@e2e.test> 100,100\n", i, i))
	}

	outq := func(want string) {
		t.Helper()
		p := s.Peer(t)
		defer expect(t, p, 205, "QUIT")
		line := expect(t, p, 290, "OUTQ")
		if !strings.HasSuffix(line, want) {
			t.Errorf("OUTQ: got %q, want ...%s", line, want)
		}
	}

	outq("qfile-backlog=3 arts=6 now-running=0")

	// what dnewslink does on the file it sends: a write lock on all of it
	f, err := os.OpenFile(s.Dir+"/dqueue/peer.S00001", os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	lk := syscall.Flock_t{Type: syscall.F_WRLCK, Whence: 0, Start: 0, Len: 0}
	if err := syscall.FcntlFlock(f.Fd(), syscall.F_SETLK, &lk); err != nil {
		t.Fatalf("lock peer.S00001: %v", err)
	}
	outq("qfile-backlog=3 arts=6 now-running=1")
}

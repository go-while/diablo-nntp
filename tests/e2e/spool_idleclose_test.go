package e2e

import (
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
)

// deletedSpoolFds returns the open descriptors of the processes below pid (diablo's feed
// children) that point at a removed file under spool (Linux: /proc/<pid>/fd).
func deletedSpoolFds(t *testing.T, pid int, spool string) []string {
	t.Helper()
	var found []string
	ents, _ := os.ReadDir("/proc")
	for _, e := range ents {
		p, err := strconv.Atoi(e.Name())
		if err != nil || p == pid {
			continue
		}
		st, err := os.ReadFile(filepath.Join("/proc", e.Name(), "stat"))
		if err != nil {
			continue
		}
		// ppid is the 4th field, after "pid (comm) state"
		f := strings.Fields(string(st[strings.LastIndexByte(string(st), ')')+1:]))
		if len(f) < 2 || f[1] != strconv.Itoa(pid) {
			continue
		}
		fds, _ := os.ReadDir(filepath.Join("/proc", e.Name(), "fd"))
		for _, fd := range fds {
			l, err := os.Readlink(filepath.Join("/proc", e.Name(), "fd", fd.Name()))
			if err == nil && strings.HasPrefix(l, spool) && strings.HasSuffix(l, " (deleted)") {
				found = append(found, e.Name()+": "+l)
			}
		}
	}
	return found
}

// TestFeederIdleClosesRemovedSpoolFiles: a feed connection that goes quiet closes the spool
// files it has open, so the space of the ones dexpire removed is freed. The files were only
// flushed when an article came in: a quiet connection (a peer backing off from a full disk)
// kept them open, and a disk that dexpire had cleaned stayed full. After the idle close the
// same connection still takes articles.
func TestFeederIdleClosesRemovedSpoolFiles(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("reads /proc/<pid>/fd (Linux)")
	}
	s := NewSite(t, SiteOptions{})
	spool := filepath.Join(s.Dir, "spool", "news")
	p := s.Peer(t)
	first := msgID(t)
	expect(t, p, 335, "IHAVE %s", first)
	send(t, p, wire(newArticle(first, "e2e.test", "idle close first", "body")))
	status(t, p, 235, "IHAVE first")

	// what dexpire does to an expired directory
	dirs, _ := filepath.Glob(filepath.Join(spool, "D.*"))
	if len(dirs) == 0 {
		t.Fatal("no spool directory after the first article")
	}
	for _, d := range dirs {
		if err := os.RemoveAll(d); err != nil {
			t.Fatal(err)
		}
	}
	if got := deletedSpoolFds(t, s.Proc.Pid(), spool); len(got) == 0 {
		t.Fatal("the feed child has no removed spool file open after the removal; the test does not see what it should")
	}

	// the connection stays quiet: the removed files must be closed
	var got []string
	waitFor(t, 20*time.Second, "the removed spool files closed", func() bool {
		got = deletedSpoolFds(t, s.Proc.Pid(), spool)
		return len(got) == 0
	})

	second := msgID(t)
	expect(t, p, 335, "IHAVE %s", second)
	send(t, p, wire(newArticle(second, "e2e.test", "idle close second", "body")))
	status(t, p, 235, "IHAVE after the idle close")
	expect(t, p, 223, "STAT %s", second)
	expect(t, p, 205, "QUIT")
}

package e2e

// Realtime dnewslink with connlife (dnewsfeeds "connlife N", dnewslink -K N; PR #9 of
// jrehmer/diablo-nntp): one TCP session across queue rotations, retirement after N seconds,
// an immediate replacement (dspoolout -J), and no descriptor growth across replacements.

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/go-while/nntp-testkit/fakeserver"
)

// supportsConnlife reports whether this dnewslink knows -K (connlife).
func supportsConnlife(s *Site) bool {
	out, _ := s.TryRun("dnewslink") // no arguments: usage, exit status 1
	return strings.Contains(out, "-K #")
}

// linkBin makes path_home/dbin point at DIABLO_BIN: dspoolout runs dicmd and dnewslink from
// there.
func linkBin(tb testing.TB, s *Site) {
	tb.Helper()
	if err := os.Symlink(s.Bin, filepath.Join(s.Dir, "dbin")); err != nil {
		tb.Fatal(err)
	}
}

// sitePids returns the pids of the running processes called name whose environment has this
// site's DIABLO_CONFIG_PATH, in ascending order. Linux only (/proc).
func sitePids(s *Site, name string) []int {
	var pids []int
	want := []byte(s.Env[0])
	entries, _ := os.ReadDir("/proc")
	for _, e := range entries {
		pid, err := strconv.Atoi(e.Name())
		if err != nil {
			continue
		}
		comm, err := os.ReadFile(filepath.Join("/proc", e.Name(), "comm"))
		if err != nil || strings.TrimSpace(string(comm)) != name {
			continue
		}
		env, err := os.ReadFile(filepath.Join("/proc", e.Name(), "environ"))
		if err != nil || !slices.ContainsFunc(bytes.Split(env, []byte{0}), func(v []byte) bool { return bytes.Equal(v, want) }) {
			continue
		}
		pids = append(pids, pid)
	}
	slices.Sort(pids)
	return pids
}

// fdCount returns the number of open descriptors of pid, or -1.
func fdCount(pid int) int {
	fds, err := os.ReadDir(filepath.Join("/proc", strconv.Itoa(pid), "fd"))
	if err != nil {
		return -1
	}
	return len(fds)
}

// realtimeWorker returns the pid of this site's realtime dnewslink (an argument -r or -r-1),
// or 0.
func realtimeWorker(s *Site) int {
	for _, pid := range sitePids(s, "dnewslink") {
		cmd, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "cmdline"))
		if err != nil {
			continue
		}
		for _, arg := range bytes.Split(cmd, []byte{0}) {
			if bytes.HasPrefix(arg, []byte("-r")) {
				return pid
			}
		}
	}
	return 0
}

// sockets returns the socket inodes ("socket:[N]") among the open descriptors of pid.
func sockets(pid int) []string {
	dir := filepath.Join("/proc", strconv.Itoa(pid), "fd")
	fds, _ := os.ReadDir(dir)
	var socks []string
	for _, fd := range fds {
		if l, err := os.Readlink(filepath.Join(dir, fd.Name())); err == nil && strings.HasPrefix(l, "socket:") {
			socks = append(socks, l)
		}
	}
	slices.Sort(socks)
	return socks
}

// fdTargets returns "fd -> target" for the open descriptors of pid, without spool article
// files: dnewslink keeps a few of those open as a cache, and their number varies.
func fdTargets(pid int) []string {
	dir := filepath.Join("/proc", strconv.Itoa(pid), "fd")
	fds, _ := os.ReadDir(dir)
	var out []string
	for _, fd := range fds {
		l, _ := os.Readlink(filepath.Join(dir, fd.Name()))
		if strings.Contains(l, "/spool/news/") {
			continue
		}
		out = append(out, fd.Name()+" -> "+l)
	}
	return out
}

// TestDnewslinkConnlife: with connlife 8, the realtime dnewslink keeps its TCP session to the
// peer across a queue rotation; after 8 seconds it retires at the next rotation and a new
// realtime worker takes over at once; the third worker holds no more descriptors than the
// first, and every article reaches the peer once, in order.
// Batch workers that dspoolout starts for rotated queue files are not counted.
func TestDnewslinkConnlife(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("reads /proc")
	}
	peer, got := newPeer(t, &fakeserver.PeerOptions{})
	feeds := GlobalLabel + `label peer
    inhost ` + PeerIP + `
    alias e2e.peer
    addgroup *
end
label ` + OutLabel + `
    hostname 127.0.0.1
    port ` + strconv.Itoa(peer.Port()) + `
    alias e2e.fake
    addgroup *
    realtime on
    rtflush
    connlife 8
end
`
	s := NewSite(t, SiteOptions{Newsfeeds: feeds, Setup: linkBin})
	if !supportsConnlife(s) {
		t.Skip("dnewslink has no -K (connlife)")
	}
	t.Cleanup(func() {
		// Realtime workers don't exit by themselves, and one retiring right now starts a
		// replacement through dspoolout -J: kill until none is left for a while.
		quiet := 0
		for range 40 {
			pids := append(sitePids(s, "dnewslink"), sitePids(s, "dspoolout")...)
			if len(pids) == 0 {
				if quiet++; quiet == 3 {
					return
				}
			} else {
				quiet = 0
			}
			for _, pid := range pids {
				_ = syscall.Kill(pid, syscall.SIGKILL)
			}
			time.Sleep(500 * time.Millisecond)
		}
		t.Errorf("dnewslink workers still running after cleanup: %v", sitePids(s, "dnewslink"))
	})
	accepted := func(id string) {
		t.Helper()
		waitFor(t, 15*time.Second, "the peer to accept "+id, func() bool { return slices.Contains(got.list(), id) })
	}

	// The first article creates dqueue/fake; dspoolout rotates it (a batch worker sends it)
	// and starts the realtime worker on the new active queue.
	a := s.Inject(t, "e2e.test", 1)
	s.waitQueue(t, OutLabel, 1)
	s.Run(t, "dspoolout")
	accepted(a[0])
	var w1 int
	var sock1 []string
	waitFor(t, 10*time.Second, "the realtime worker connected", func() bool {
		w1 = realtimeWorker(s)
		if w1 > 0 {
			sock1 = sockets(w1)
		}
		return w1 > 0 && len(sock1) > 0
	})
	start := time.Now()
	b := s.Inject(t, "e2e.test", 1) // through the realtime worker
	accepted(b[0])

	// A rotation within the connlife: same worker, same socket, next article.
	s.Run(t, "dspoolout")
	c := s.Inject(t, "e2e.test", 1)
	accepted(c[0])
	if since := time.Since(start); since < 7*time.Second {
		if w := realtimeWorker(s); w != w1 {
			t.Errorf("realtime worker changed across the rotation after %v: %d -> %d", since.Round(time.Millisecond), w1, w)
		} else if sk := sockets(w); !slices.Equal(sk, sock1) {
			t.Errorf("realtime worker reconnected across the rotation: sockets %v -> %v", sock1, sk)
		}
	} else {
		t.Logf("rotation step took %v, too long to check the session", since)
	}

	// After connlife, a worker that has delivered records retires at the next rotation, which
	// it notices once diablo has created the new active queue file with the next article (as on
	// a live feed; cron's dspoolout does the rotation). A new realtime worker takes over at once.
	cycle := func(prev int, since time.Time, n int) (int, string) {
		t.Helper()
		if d := 9*time.Second - time.Since(since); d > 0 {
			time.Sleep(d)
		}
		s.Run(t, "dspoolout")
		id := s.Inject(t, "e2e.test", 1)[0]
		accepted(id)
		var w int
		waitFor(t, 10*time.Second, "realtime worker "+strconv.Itoa(n)+" after connlife, a rotation and an article", func() bool {
			w = realtimeWorker(s)
			return w > 0 && w != prev
		})
		return w, id
	}
	fdl1 := fdTargets(w1)
	w2, d := cycle(w1, start, 2)
	fdl2 := fdTargets(w2)
	w3, e := cycle(w2, time.Now(), 3)
	fdl3 := fdTargets(w3)
	w4, f := cycle(w3, time.Now(), 4)
	fdl4 := fdTargets(w4)
	for i, l := range [][]string{fdl2, fdl3, fdl4} {
		if len(l) > len(fdl1) {
			t.Errorf("descriptors grew across replacements:\n worker 1: %q\n worker %d: %q", fdl1, i+2, l)
		}
	}
	if want := []string{a[0], b[0], c[0], d, e, f}; !slices.Equal(got.list(), want) {
		t.Errorf("peer accepted %q, want %q", got.list(), want)
	}
}

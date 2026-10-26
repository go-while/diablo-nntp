package e2e

import (
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

// TestDnewslinkConnlifeNearest: a connlife worker that carries traffic retires at the queue
// rotation nearest its connlife, not at the first rotation after it. With connlife 12, the
// replacement worker started just after a rotation is retired at a rotation 9 s later: the
// next one would be about 18 s in, farther from 12. Before #30 it was kept there, because it
// was short of 12 s, and served until the rotation after (connlife 60 gave about 120 s with
// cron's once-a-minute rotation). dnewslink counts whole seconds, so the times leave a
// second of margin either way. The site logs to log/general.log, shown when the test fails.
// Linux only (reads /proc).
func TestDnewslinkConnlifeNearest(t *testing.T) {
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
    connlife 12
end
`
	s := NewSite(t, SiteOptions{Newsfeeds: feeds, Setup: linkBin,
		Config: map[string]string{"path_generallog": "%s/general.log"}})
	t.Cleanup(func() {
		if !t.Failed() {
			return
		}
		b, _ := os.ReadFile(filepath.Join(s.Dir, "log", "general.log"))
		for _, l := range strings.Split(string(b), "\n") {
			if strings.Contains(l, "fake:") || strings.Contains(l, "connlife") || strings.Contains(l, "dspoolout") {
				t.Log("log: " + l)
			}
		}
	})
	if !supportsConnlife(s) {
		t.Skip("dnewslink has no -K (connlife)")
	}
	t.Cleanup(func() {
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
	})
	accepted := func(id string) {
		t.Helper()
		waitFor(t, 15*time.Second, "the peer to accept "+id, func() bool { return slices.Contains(got.list(), id) })
	}
	// rotate: a queue rotation as cron's dspoolout does it, noticed by the realtime worker
	// with the next article. Returns when the peer has the article.
	rotate := func() {
		t.Helper()
		s.Run(t, "dspoolout")
		accepted(s.Inject(t, "e2e.test", 1)[0])
	}
	sleepUntil := func(at time.Time) {
		if d := time.Until(at); d > 0 {
			time.Sleep(d)
		}
	}
	newWorker := func(prev int, within time.Duration, what string) int {
		t.Helper()
		var w int
		waitFor(t, within, what, func() bool {
			w = realtimeWorker(s)
			return w > 0 && w != prev
		})
		return w
	}

	// Worker 1: started by the first rotation, retired at a rotation 13 s later (either rule).
	a := s.Inject(t, "e2e.test", 1)
	s.waitQueue(t, OutLabel, 1)
	s.Run(t, "dspoolout")
	accepted(a[0])
	w1 := newWorker(0, 10*time.Second, "the first realtime worker")
	start1 := time.Now()
	// An article through worker 1, as on a live feed: with its active queue still empty at
	// connlife it would retire idle instead, without a rotation.
	accepted(s.Inject(t, "e2e.test", 1)[0])
	sleepUntil(start1.Add(13 * time.Second))
	rot := time.Now()
	rotate()
	w2 := newWorker(w1, 10*time.Second, "the replacement after connlife")
	t.Logf("worker 2 (pid %d) started %v after the rotation", w2, time.Since(rot).Round(time.Millisecond))

	// 9 s after its start rotation: nearer to connlife (12) than the next rotation (about
	// 18 s), so worker 2 retires here.
	sleepUntil(rot.Add(9 * time.Second))
	rotate()
	var w3 int
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if w := realtimeWorker(s); w > 0 && w != w2 {
			w3 = w
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if w3 == 0 {
		t.Fatalf("worker 2 still serving %v after its start rotation (connlife 12): not retired at the rotation nearest connlife", time.Since(rot).Round(time.Millisecond))
	}
	t.Logf("worker 2 retired after %v (connlife 12)", time.Since(rot).Round(time.Millisecond))
}

package e2e

import (
	"fmt"
	testkit "github.com/go-while/nntp-testkit"
	"github.com/go-while/nntp-testkit/fakeserver"
	"os"
	"path/filepath"
	"strconv"
	"syscall"
	"testing"
	"time"
)

// newHoldPeer is a peer that accepts connections and never answers (no greeting, no reply), so
// every dnewslink that connects stays connected. Its Open, MaxOpen and Accepted count the
// connections open now, the most seen at once and all accepted.
func newHoldPeer(tb testing.TB) *fakeserver.Server {
	tb.Helper()
	return fakeserver.New(tb, fakeserver.Options{NoGreeting: true})
}

// TestMaxParallelCountsRealtime: maxparallel is the number of connections to the host, and a
// realtime label's realtime dnewslink is one of them. With maxparallel 2, a running realtime
// worker and a backlog of five queue files, dspoolout starts one batch worker, not two. Before,
// the realtime worker came on top of maxparallel: four hashfeed labels with maxparallel 2 opened
// up to 12 connections to a peer that allows 10.
func TestMaxParallelCountsRealtime(t *testing.T) {
	requireProcessInspection(t, true)
	peer := newHoldPeer(t)
	feeds := GlobalLabel + `label ` + OutLabel + `
    hostname 127.0.0.1
    port ` + strconv.Itoa(peer.Port()) + `
    alias e2e.fake
    addgroup *
    realtime on
    maxparallel 2
end
`
	s := NewSite(t, SiteOptions{Newsfeeds: feeds, Setup: linkBin})
	t.Cleanup(func() {
		for range 20 {
			pids := append(sitePids(t, s, "dnewslink"), sitePids(t, s, "dspoolout")...)
			if len(pids) == 0 {
				return
			}
			for _, pid := range pids {
				_ = syscall.Kill(pid, syscall.SIGKILL)
			}
			time.Sleep(250 * time.Millisecond)
		}
	})
	logWorkerInspectionOnFailure(t, s)

	// One queued entry: dspoolout moves it to fake.S00000 for a batch worker and starts the
	// realtime worker on a new dqueue/fake. Both connect and stay (the peer never answers).
	q := filepath.Join(s.Dir, "dqueue")
	entry := func(i int) []byte {
		return []byte(fmt.Sprintf("D.00000000/B.0000 0,100 <backlog%d@e2e.test>\n", i))
	}
	if err := os.WriteFile(filepath.Join(q, OutLabel), entry(0), 0o644); err != nil {
		t.Fatal(err)
	}
	s.Run(t, "dspoolout")
	testkit.WaitFor(t, 10*time.Second, "the realtime and the first batch worker connected", func() bool {
		open := peer.Open()
		return open == 2 && realtimeWorker(t, s) > 0
	})

	// Five more queue files behind it; the sequence file's time is old, so dspoolout runs
	// the backlog now.
	for i := 1; i <= 5; i++ {
		if err := os.WriteFile(filepath.Join(q, fmt.Sprintf("%s.S%05d", OutLabel, i)), entry(i), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(q, "."+OutLabel+".seq"), []byte(fmt.Sprintf("%5d %5d %08x\n", 0, 6, 0)), 0o644); err != nil {
		t.Fatal(err)
	}
	s.Run(t, "dspoolout")
	time.Sleep(4 * time.Second) // time for another batch worker to connect, if one starts
	open, most, accepted := peer.Open(), peer.MaxOpen(), peer.Accepted()
	if most > 2 {
		t.Errorf("up to %d connections to the peer with maxparallel 2 (open now %d, accepted %d); want at most 2, the realtime worker included", most, open, accepted)
	}
}

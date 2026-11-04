package e2e

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"sync"
	"syscall"
	"testing"
	"time"
)

// holdPeer accepts connections and never answers, so every dnewslink that connects stays
// connected. It reports the number of connections open now and the most seen at once.
type holdPeer struct {
	ln       net.Listener
	mu       sync.Mutex
	open     int
	max      int
	accepted int
}

func newHoldPeer(tb testing.TB) *holdPeer {
	tb.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		tb.Fatal(err)
	}
	p := &holdPeer{ln: ln}
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			p.mu.Lock()
			p.open++
			p.accepted++
			p.max = max(p.max, p.open)
			p.mu.Unlock()
			go func() {
				buf := make([]byte, 512)
				for {
					if _, err := c.Read(buf); err != nil {
						break
					}
				}
				c.Close()
				p.mu.Lock()
				p.open--
				p.mu.Unlock()
			}()
		}
	}()
	tb.Cleanup(func() { ln.Close() })
	return p
}

func (p *holdPeer) port() int { return p.ln.Addr().(*net.TCPAddr).Port }

func (p *holdPeer) counts() (open, most, accepted int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.open, p.max, p.accepted
}

// TestMaxParallelCountsRealtime: maxparallel is the number of connections to the host, and a
// realtime label's realtime dnewslink is one of them. With maxparallel 2, a running realtime
// worker and a backlog of five queue files, dspoolout starts one batch worker, not two. Before,
// the realtime worker came on top of maxparallel: four hashfeed labels with maxparallel 2 opened
// up to 12 connections to a peer that allows 10.
func TestMaxParallelCountsRealtime(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("stops the workers through /proc")
	}
	peer := newHoldPeer(t)
	feeds := GlobalLabel + `label ` + OutLabel + `
    hostname 127.0.0.1
    port ` + strconv.Itoa(peer.port()) + `
    alias e2e.fake
    addgroup *
    realtime on
    maxparallel 2
end
`
	s := NewSite(t, SiteOptions{Newsfeeds: feeds, Setup: linkBin})
	t.Cleanup(func() {
		for range 20 {
			pids := append(sitePids(s, "dnewslink"), sitePids(s, "dspoolout")...)
			if len(pids) == 0 {
				return
			}
			for _, pid := range pids {
				_ = syscall.Kill(pid, syscall.SIGKILL)
			}
			time.Sleep(250 * time.Millisecond)
		}
	})

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
	waitFor(t, 10*time.Second, "the realtime and the first batch worker connected", func() bool {
		open, _, _ := peer.counts()
		return open == 2 && realtimeWorker(s) > 0
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
	open, most, accepted := peer.counts()
	if most > 2 {
		t.Errorf("up to %d connections to the peer with maxparallel 2 (open now %d, accepted %d); want at most 2, the realtime worker included", most, open, accepted)
	}
}

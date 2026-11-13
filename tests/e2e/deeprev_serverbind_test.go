package e2e

import (
	"strconv"
	"testing"
	"time"
)

// TestDeepRevServerBindFailureLeaksSocket (L4-02): AddServer() (dreaderd/server.c) creates
// the socket for a dserver.hosts entry and returned without closing it when the entry's
// "bind=" address could not be resolved or bound. A failed entry makes the reader read
// dserver.hosts again a few seconds later, so each reader process lost one descriptor every
// five seconds or so for as long as the address was not there (an interface that is down, a
// typo), until it ran into its descriptor limit.
//
// A spool entry with bind=192.0.2.77 (TEST-NET-1, not an address of this host) next to a
// working one: the reader's descriptor count must not grow over 30 seconds.
func TestDeepRevServerBindFailureLeaksSocket(t *testing.T) {
	requireProcessInspection(t, true)
	_, spool := spoolStore(t)
	port := strconv.Itoa(spool.Port())
	hosts := "127.0.0.1\ts1p" + port + "\n" +
		"127.0.0.9\ts2p" + port + "\tbind=192.0.2.77\n"
	s := NewReaderSite(t, ReaderOptions{Spool: spool, Files: map[string]string{"dserver.hosts": hosts}})

	c := s.Reader(t)
	// Commands keep the reader's loop turning: it counts its timers every tenth pass.
	busy := func(d time.Duration) {
		limit := time.Now().Add(d)
		for time.Now().Before(limit) {
			c.ExpectCmd(111, "DATE")
			time.Sleep(100 * time.Millisecond)
		}
	}
	busy(3 * time.Second) // the first attempt on the bad entry is over

	count := func() map[int]int {
		m := map[int]int{}
		for _, pid := range sitePids(t, s, "dreaderd") {
			m[pid] = fdCount(t, pid)
		}
		return m
	}
	before := count()
	busy(30 * time.Second)
	after := count()
	c.ExpectCmd(205, "QUIT")

	for pid, n := range before {
		if m, ok := after[pid]; ok && n >= 0 && m > n+2 {
			t.Errorf("dreaderd process %d: %d descriptors, 30 s later %d: one is lost for each attempt on the entry with bind=192.0.2.77",
				pid, n, m)
		}
	}
}

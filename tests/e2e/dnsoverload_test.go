package e2e

// dreaderd under a login flood with slow DNS: new connections that find every DNS worker
// busy are queued briefly or refused at once with 400, and AUTHINFO logins of greeted
// clients go ahead of the queued new connections. The tests preload the resolver shim
// (testdata/dnsshim) with a lookup delay, so they run on Linux only.

import (
	"bufio"
	"fmt"
	"net"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

// floodAccess: fred must log in from 127.0.0.1 (as AuthReaderAccess); the flood clients
// from 127.1.0.0/16 are greeted and need no login.
const floodAccess = `vserverdef DEFAULT
    hostname ` + PathHost + `
end
authdef fred
    user fred
    pass fredspass
end
readerdef userfred
    read yes
    post yes
    auth fred
end
readerdef rp
    read yes
end
access 127.0.0.1 userfred
access 127.1.0.0/16 rp
`

// floodResult is the outcome of one flood client: the first reply line (or an error
// text) and the time from the start of the dial to it.
type floodResult struct {
	line string
	wait time.Duration
}

// flood opens n connections from distinct 127.1.x.y addresses, spread over spread, and
// reads the first line of each. Every address is new to the DNS cache.
func flood(t *testing.T, addr string, n int, spread time.Duration) []floodResult {
	t.Helper()
	res := make([]floodResult, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			time.Sleep(spread * time.Duration(i) / time.Duration(n))
			ip := fmt.Sprintf("127.1.%d.%d", 1+i/250, 1+i%250)
			d := net.Dialer{LocalAddr: &net.TCPAddr{IP: net.ParseIP(ip)}, Timeout: 60 * time.Second}
			t0 := time.Now()
			c, err := d.Dial("tcp", addr)
			if err != nil {
				res[i] = floodResult{"error: " + err.Error(), time.Since(t0)}
				return
			}
			defer c.Close()
			c.SetReadDeadline(time.Now().Add(60 * time.Second))
			l, err := bufio.NewReader(c).ReadString('\n')
			res[i].wait = time.Since(t0)
			if err != nil {
				res[i].line = "error: " + err.Error()
				return
			}
			res[i].line = strings.TrimSpace(l)
			fmt.Fprintf(c, "QUIT\r\n")
		}(i)
	}
	wg.Wait()
	return res
}

// countFlood checks that every flood client got a greeting or a 400 within bound, and
// returns how many got each.
func countFlood(t *testing.T, res []floodResult, bound time.Duration) (greeted, busy int) {
	t.Helper()
	var slowGreet, slowBusy time.Duration
	quickBusy := 0
	for i, r := range res {
		switch {
		case strings.HasPrefix(r.line, "200"), strings.HasPrefix(r.line, "201"):
			greeted++
			slowGreet = max(slowGreet, r.wait)
		case strings.HasPrefix(r.line, "400"):
			busy++
			slowBusy = max(slowBusy, r.wait)
			if r.wait < 500*time.Millisecond {
				quickBusy++
			}
		default:
			t.Errorf("client %d: got %q after %v, want a greeting or 400", i, r.line, r.wait)
			continue
		}
		if r.wait > bound {
			t.Errorf("client %d: %q after %v, want it within %v", i, r.line, r.wait, bound)
		}
	}
	t.Logf("%d clients: %d greeted (slowest %v), %d got 400 (%d within 0.5 s, slowest %v)",
		len(res), greeted, slowGreet, busy, quickBusy, slowBusy)
	return greeted, busy
}

// startFloodReader starts a reader with one DNS worker, the shim delaying every faked
// lookup by delayMs, and room for many sessions.
func startFloodReader(t *testing.T, delayMs int, conf map[string]string) *Site {
	t.Helper()
	sh := buildDNSShim(t)
	_, spool := spoolStore(t)
	c := map[string]string{"readerthreads": "64"}
	for k, v := range conf {
		c[k] = v
	}
	s := NewReaderSite(t, ReaderOptions{Spool: spool, Access: floodAccess, Config: c, NoStart: true})
	s.Env = append(s.Env, "LD_PRELOAD="+sh.so, fmt.Sprintf("DSHIM_DELAY_MS=%d", delayMs))
	_, port, _ := net.SplitHostPort(s.Addr)
	startReader(t, s, "[127.0.0.1]:"+port)
	return s
}

// TestReaderDNSOverload400: a burst of 150 new clients against one DNS worker that needs
// 200 ms per login (30 s of work). Each client gets a greeting or a quick "400 Server too
// busy" within the default readerdnswait (5 s) plus slack, and none is reset. Fails on main:
// dreaderd stopped accepting while its worker was busy, so the clients waited in the kernel
// backlog for up to 30 s.
func TestReaderDNSOverload400(t *testing.T) {
	s := startFloodReader(t, 100, nil)
	greeted, busy := countFlood(t, flood(t, s.Addr, 150, 500*time.Millisecond), 12*time.Second)
	if greeted == 0 || busy == 0 {
		t.Errorf("%d greeted and %d got 400, want some of each", greeted, busy)
	}
	c := s.Reader(t)
	expect(t, c, 381, "AUTHINFO USER fred")
	expect(t, c, 281, "AUTHINFO PASS fredspass")
	expect(t, c, 205, "QUIT")
}

// TestReaderDNSOverloadAuthFirst: 20 greeted clients send AUTHINFO PASS together while a
// flood of new clients keeps the only DNS worker busy. Every login gets 281 within 1.5 s:
// AUTHINFO logins wait ahead of the queued new connections and have no queue limit. Fails
// on main: the queue held 4 * readerdns + 4 requests, so most of the AUTHINFO PASS commands
// got "400 Server too busy".
func TestReaderDNSOverloadAuthFirst(t *testing.T) {
	s := startFloodReader(t, 100, nil)
	const n = 20
	conns := make([]net.Conn, n)
	rds := make([]*bufio.Reader, n)
	line := func(i int) string {
		conns[i].SetReadDeadline(time.Now().Add(60 * time.Second))
		l, err := rds[i].ReadString('\n')
		if err != nil {
			return "error: " + err.Error()
		}
		return strings.TrimSpace(l)
	}
	for i := 0; i < n; i++ {
		c, err := net.DialTimeout("tcp", s.Addr, 20*time.Second)
		if err != nil {
			t.Fatal(err)
		}
		defer c.Close()
		conns[i], rds[i] = c, bufio.NewReader(c)
		if g := line(i); !strings.HasPrefix(g, "200") && !strings.HasPrefix(g, "201") {
			t.Fatalf("client %d: greeting %q", i, g)
		}
		fmt.Fprintf(c, "AUTHINFO USER fred\r\n")
		if r := line(i); !strings.HasPrefix(r, "381") {
			t.Fatalf("client %d: AUTHINFO USER: %q", i, r)
		}
	}

	var fres []floodResult
	done := make(chan struct{})
	go func() {
		fres = flood(t, s.Addr, 100, 2*time.Second)
		close(done)
	}()
	time.Sleep(time.Second) // let new connections pile up behind the worker

	res := make([]string, n)
	waits := make([]time.Duration, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			t0 := time.Now()
			fmt.Fprintf(conns[i], "AUTHINFO PASS fredspass\r\n")
			res[i] = line(i)
			waits[i] = time.Since(t0)
			fmt.Fprintf(conns[i], "QUIT\r\n")
		}(i)
	}
	wg.Wait()
	// Behind the queued new connections a login would wait about half of readerdnswait
	// (2.5 s); ahead of them, one worker request (0.2 s) plus the logins before it.
	for i, r := range res {
		if !strings.HasPrefix(r, "281") {
			t.Errorf("greeted client %d: AUTHINFO PASS got %q, want 281", i, r)
		} else if waits[i] > 1500*time.Millisecond {
			t.Errorf("greeted client %d: 281 after %v, want it within 1.5 s (ahead of the new connections)",
				i, waits[i])
		}
	}
	t.Logf("slowest AUTHINFO PASS reply after %v", slices.Max(waits))
	<-done
	countFlood(t, fres, 12*time.Second)
}

// TestReaderDNSQueueOptions: readerdnsqueue 0 queues no new connection, so with the worker
// busy the burst gets 400 at once, and readerdnswait bounds the wait of the queued ones.
func TestReaderDNSQueueOptions(t *testing.T) {
	t.Run("queue0", func(t *testing.T) {
		s := startFloodReader(t, 250, map[string]string{"readerdnsqueue": "0"})
		greeted, busy := countFlood(t, flood(t, s.Addr, 30, 0), 5*time.Second)
		if greeted == 0 || busy < 20 {
			t.Errorf("%d greeted and %d got 400, want at least 1 and 20", greeted, busy)
		}
	})
	t.Run("wait1", func(t *testing.T) {
		s := startFloodReader(t, 100, map[string]string{"readerdnswait": "1"})
		greeted, busy := countFlood(t, flood(t, s.Addr, 60, 300*time.Millisecond), 4*time.Second)
		if greeted == 0 || busy == 0 {
			t.Errorf("%d greeted and %d got 400, want some of each", greeted, busy)
		}
	})
}

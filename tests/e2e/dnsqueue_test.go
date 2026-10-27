package e2e

import (
	"bufio"
	"fmt"
	"net"
	"strings"
	"sync"
	"testing"
	"time"
)

// authBurst opens n connections to the reader at once, each doing AUTHINFO USER fred, then
// all send AUTHINFO PASS together, and returns the first reply code of each login ("281",
// "400", ...), or an error text where the connection failed.
func authBurst(t *testing.T, addr string, n int) []string {
	t.Helper()
	res := make([]string, n)
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
	var wg sync.WaitGroup
	step := func(f func(i int)) {
		for i := 0; i < n; i++ {
			wg.Add(1)
			go func(i int) { defer wg.Done(); f(i) }(i)
		}
		wg.Wait()
	}
	step(func(i int) {
		c, err := net.DialTimeout("tcp", addr, 20*time.Second)
		if err != nil {
			res[i] = "error: " + err.Error()
			return
		}
		conns[i], rds[i] = c, bufio.NewReader(c)
		if g := line(i); !strings.HasPrefix(g, "200") && !strings.HasPrefix(g, "201") {
			res[i] = g
			return
		}
		fmt.Fprintf(c, "AUTHINFO USER fred\r\n")
		if r := line(i); !strings.HasPrefix(r, "381") {
			res[i] = r
		}
	})
	step(func(i int) {
		if res[i] != "" {
			return
		}
		fmt.Fprintf(conns[i], "AUTHINFO PASS fredspass\r\n")
		res[i] = line(i)
		fmt.Fprintf(conns[i], "QUIT\r\n")
	})
	for _, c := range conns {
		if c != nil {
			c.Close()
		}
	}
	return res
}

// TestReaderLoginsQueueForDNSWorker (D-51): with one DNS/auth worker (readerdns 1), a burst
// of logins at the same time waits for the worker instead of getting "400 Server too busy".
// Fails on main before the D-51 fix: some of the AUTHINFO PASS commands got 400.
func TestReaderLoginsQueueForDNSWorker(t *testing.T) {
	_, spool := spoolStore(t)
	// readerthreads 16: the test reader serves 4 sessions by default, and the burst keeps all
	// of its logins open until every one has sent AUTHINFO PASS.
	s := NewReaderSite(t, ReaderOptions{Spool: spool, Access: AuthReaderAccess,
		Config: map[string]string{"readerthreads": "16"}})
	for i, r := range authBurst(t, s.Addr, 8) {
		if !strings.HasPrefix(r, "281") {
			t.Errorf("login %d: got %q, want 281", i, r)
		}
	}
}

// TestReaderDNSQueueLimit (D-51): when more logins arrive than the queue holds (4 * readerdns
// + 4), the extra ones are turned away with 400 and the rest still succeed; nothing hangs, and
// the reader serves a login afterwards.
func TestReaderDNSQueueLimit(t *testing.T) {
	_, spool := spoolStore(t)
	s := NewReaderSite(t, ReaderOptions{Spool: spool, Access: AuthReaderAccess,
		Config: map[string]string{"readerthreads": "64"}})
	ok, busy := 0, 0
	for i, r := range authBurst(t, s.Addr, 40) {
		switch {
		case strings.HasPrefix(r, "281"):
			ok++
		case strings.HasPrefix(r, "400"):
			busy++
		default:
			t.Errorf("login %d: got %q, want 281 or 400", i, r)
		}
	}
	t.Logf("%d logins succeeded, %d got 400", ok, busy)
	// One worker and a queue of 8 hold 9 logins at a time, so at least 9 get 281 and, of 40,
	// some get 400.
	if ok < 9 || busy == 0 {
		t.Errorf("%d logins succeeded and %d got 400, want at least 9 and at least 1", ok, busy)
	}
	c := s.Reader(t)
	expect(t, c, 381, "AUTHINFO USER fred")
	expect(t, c, 281, "AUTHINFO PASS fredspass")
	expect(t, c, 205, "QUIT")
}

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
	return authBurstWith(t, addr, n, func(int) string { return "fredspass" })
}

// authBurstWith is authBurst with a per-connection password.
func authBurstWith(t *testing.T, addr string, n int, pass func(i int) string) []string {
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
		fmt.Fprintf(conns[i], "AUTHINFO PASS %s\r\n", pass(i))
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

// TestReaderDNSQueueLimit (D-51): 40 logins at once against one DNS/auth worker. AUTHINFO
// logins of greeted clients have no queue limit (their number is bounded by the connection
// limit), so each gets 281, nothing hangs, and the reader serves a login afterwards. Before
// the overload change the queue held 4 * readerdns + 4 requests and the rest got 400.
func TestReaderDNSQueueLimit(t *testing.T) {
	_, spool := spoolStore(t)
	s := NewReaderSite(t, ReaderOptions{Spool: spool, Access: AuthReaderAccess,
		Config: map[string]string{"readerthreads": "64"}})
	for i, r := range authBurst(t, s.Addr, 40) {
		if !strings.HasPrefix(r, "281") {
			t.Errorf("login %d: got %q, want 281", i, r)
		}
	}
	c := s.Reader(t)
	c.ExpectCmd(381, "AUTHINFO USER fred")
	c.ExpectCmd(281, "AUTHINFO PASS fredspass")
	c.ExpectCmd(205, "QUIT")
}

// TestReaderDNSQueueAuthPairing: the DNS/auth worker IPC must keep each request and reply
// together.  One worker serves bursts in which every other login uses the right password;
// each connection must get the answer for its own AUTHINFO PASS (281 or 481).  A
// desynchronised stream socketpair (the FreeBSD 0/40 stall) either pairs a reply with the
// wrong connection or stops serving altogether, and both show up here.
func TestReaderDNSQueueAuthPairing(t *testing.T) {
	_, spool := spoolStore(t)
	s := NewReaderSite(t, ReaderOptions{Spool: spool, Access: AuthReaderAccess,
		Config: map[string]string{"readerthreads": "64"}})
	// Eight at a time: none is turned away with 400 and every connection gets a verdict.
	for round := 0; round < 5; round++ {
		res := authBurstWith(t, s.Addr, 8, func(i int) string {
			if i%2 == 0 {
				return "fredspass"
			}
			return "wrongpass"
		})
		for i, r := range res {
			want := "481"
			if i%2 == 0 {
				want = "281"
			}
			if !strings.HasPrefix(r, want) {
				t.Errorf("round %d login %d: got %q, want %s for its own AUTHINFO PASS",
					round, i, r, want)
			}
		}
	}
	c := s.Reader(t)
	c.ExpectCmd(381, "AUTHINFO USER fred")
	c.ExpectCmd(281, "AUTHINFO PASS fredspass")
	c.ExpectCmd(205, "QUIT")
}

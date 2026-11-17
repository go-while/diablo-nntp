package e2e

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-while/nntp-testkit/client"
)

// diabloChildren counts the processes whose parent is pid (Linux /proc).
func diabloChildren(pid int) int {
	n := 0
	ents, _ := os.ReadDir("/proc")
	for _, e := range ents {
		if _, err := strconv.Atoi(e.Name()); err != nil {
			continue
		}
		b, err := os.ReadFile(filepath.Join("/proc", e.Name(), "stat"))
		if err != nil {
			continue
		}
		s := string(b)
		i := strings.LastIndexByte(s, ')')
		if i < 0 {
			continue
		}
		f := strings.Fields(s[i+1:])
		if len(f) > 1 && f[1] == strconv.Itoa(pid) {
			n++
		}
	}
	return n
}

// r2PtrDelayFile is preloaded into diablo: a reverse lookup (getnameinfo with NI_NAMEREQD)
// first sleeps the number of seconds in the file named by R2_PTRDELAY_FILE, as when the PTR
// zone of the connecting address answers slowly. A file, so that the running diablo can be
// switched after NewSite's readiness check.
const r2PtrDelayFile = `#define _GNU_SOURCE
#include <dlfcn.h>
#include <fcntl.h>
#include <netdb.h>
#include <stdarg.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>
#include <sys/socket.h>
/*MARK*/
int
getnameinfo(const struct sockaddr *sa, socklen_t salen, char *host, socklen_t hostlen,
	char *serv, socklen_t servlen, int flags)
{
    static int (*real)(const struct sockaddr *, socklen_t, char *, socklen_t, char *, socklen_t, int);
    const char *p = getenv("R2_PTRDELAY_FILE");
    FILE *f;
    int d = 0;

    if (real == NULL)
	real = (int (*)(const struct sockaddr *, socklen_t, char *, socklen_t, char *, socklen_t, int))
	    dlsym(RTLD_NEXT, "getnameinfo");
    if ((flags & NI_NAMEREQD) && p != NULL && (f = fopen(p, "r")) != NULL) {
	if (fscanf(f, "%d", &d) != 1)
	    d = 0;
	fclose(f);
	if (d > 0) {
	    shim_mark("ptr delay %d", d);
	    sleep(d);
	}
    }
    return real(sa, salen, host, hostlen, serv, servlen, flags);
}
`

// TestR2AcceptDNSBeforeAccess (deep review r2 M15): DoAccept() forks for every connection and
// the child does a reverse lookup and a forward lookup (lib/hostauth.c Authenticate()) with no
// time limit before it looks at dhosts.cache. Whoever answers the PTR query of the connecting
// address sets how long a fork slot and a master pipe stay taken, and a peer listed by its
// address waits for the same lookup. With reverse lookups taking 8 s and 300 strangers
// connected, the peer 127.0.0.2 (an IP inhost) must still be greeted within 3 s.
func TestR2AcceptDNSBeforeAccess(t *testing.T) {
	if !preloadHost(false) {
		t.Skip("Linux only (/proc, LD_PRELOAD)")
	}
	so := preloadShim(t, "ptrdelayfile", withMark(r2PtrDelayFile))
	mark := filepath.Join(t.TempDir(), "mark")
	delay := filepath.Join(t.TempDir(), "delay")
	s := NewSite(t, SiteOptions{Env: []string{lockFaultEnv(so), "SHIM_MARK=" + mark, "R2_PTRDELAY_FILE=" + delay}})
	t.Cleanup(func() {
		if t.Failed() {
			m := shimMarks(mark)
			if len(m) > 5 {
				m = m[:5]
			}
			t.Logf("first marks:\n%s", strings.Join(m, "\n"))
		}
	})
	if err := os.WriteFile(delay, []byte("8\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	const strangers = 300
	var wg sync.WaitGroup
	var mu sync.Mutex
	var conns []net.Conn
	for i := 0; i < strangers; i++ {
		ip := fmt.Sprintf("127.0.%d.%d", 1+i/250, 10+i%250)
		wg.Add(1)
		go func() {
			defer wg.Done()
			d := net.Dialer{LocalAddr: &net.TCPAddr{IP: net.ParseIP(ip)}, Timeout: 5 * time.Second}
			if nc, err := d.Dial("tcp", s.Addr); err == nil {
				mu.Lock()
				conns = append(conns, nc)
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	t.Cleanup(func() {
		for _, c := range conns {
			c.Close()
		}
	})
	time.Sleep(time.Second)
	kids := diabloChildren(s.Proc.Pid())
	t.Logf("%d of %d stranger connections open; diablo children now: %d; reverse lookups started: %d",
		len(conns), strangers, kids, len(shimMarked(mark, "ptr delay")))

	start := time.Now()
	d := net.Dialer{LocalAddr: &net.TCPAddr{IP: net.ParseIP(PeerIP)}, Timeout: 5 * time.Second}
	nc, err := d.Dial("tcp", s.Addr)
	if err != nil {
		t.Fatalf("peer dial: %v", err)
	}
	_ = nc.SetDeadline(time.Now().Add(20 * time.Second))
	c := client.New(t, nc)
	g, err := c.ReadGreeting()
	took := time.Since(start)
	if err != nil {
		t.Fatalf("peer greeting: %v (after %v)", err, took)
	}
	if took > 3*time.Second {
		t.Errorf("the peer %s (listed by address) waited %v for its greeting %q behind its own reverse lookup, with %d strangers holding diablo children",
			PeerIP, took.Round(100*time.Millisecond), g, kids)
	}
	c.ExpectCmd(205, "QUIT")
}

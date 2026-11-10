package e2e

import (
	"bufio"
	"fmt"
	"net"
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"

	testkit "github.com/go-while/nntp-testkit"
)

// TestDeepRevListCacheUnlockOnce: NEWGROUPS is answered from a cache of the group list, which
// a reader process builds once and shares between its connections; a connection that reads
// it holds a read lock, and the cache is rebuilt (after a new group) only when no one does.
// NNTerminate() gave the read lock back, but left the connection marked as a reader of the
// cache. When the idle and the session timeout end a connection in the same pass, it runs
// twice, and the lock count went to -1: from then on the cache was never built again (and a
// rebuild could free it under a connection that reads it).
//
// The cache answers NEWGROUPS in the order of creation time (groups of the same second in
// the reverse order), a scan of the group list in its own order: that tells the answers
// apart. A client that sends NEWGROUPS and does not read, with idletimeout and
// sessiontimeout 2, is ended by both timeouts in one pass, in the middle of its answer from
// the cache. Then a new group: the second NEWGROUPS after it must
// come from the rebuilt cache again.
func TestDeepRevListCacheUnlockOnce(t *testing.T) {
	// enough groups for an answer that does not fit into the socket buffers (txbufsize 512)
	var groups []string
	for i := 0; i < 400; i++ {
		groups = append(groups, fmt.Sprintf("e2e.lc%03d.%s", i, strings.Repeat("x", 190)))
	}
	access := `vserverdef DEFAULT
    hostname ` + PathHost + `
end
readerdef rp
    read yes
    idletimeout 2
    sessiontimeout 2
    txbufsize 512
end
access 127.0.0.1 rp
`
	_, spool := spoolStore(t)
	s := NewReaderSite(t, ReaderOptions{Spool: spool, Groups: groups, Access: access})
	t.Cleanup(func() {
		if t.Failed() {
			t.Logf("general.log:\n%s", generalLog(s))
		}
	})

	newgroups := func() []string {
		t.Helper()
		c := s.Reader(t)
		defer c.Close()
		c.ExpectCmd(231, "NEWGROUPS 000101 000000 GMT")
		var names []string
		for _, l := range c.ReadDotLines() {
			names = append(names, strings.Fields(l)[0])
		}
		c.ExpectCmd(205, "QUIT")
		return names
	}
	scan := newgroups()   // builds the cache
	cached := newgroups() // from the cache
	if len(scan) != len(groups) || slices.Equal(scan, cached) {
		t.Fatalf("the scenario did not develop: NEWGROUPS gave %d groups, and the same order from the cache as from the scan: %v",
			len(scan), slices.Equal(scan, cached))
	}

	// a client that asks and does not read, until both timeouts end it in the same pass
	count := func(what string) int { return strings.Count(generalLog(s), "timeout "+what+" 127.0.0.1") }
	both := false
	for try := 1; try <= 3 && !both; try++ {
		idle, session := count("idle"), count("session")
		d := net.Dialer{Timeout: 10 * time.Second, Control: func(_, _ string, rc syscall.RawConn) error {
			var err error
			rc.Control(func(fd uintptr) { err = syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, syscall.SO_RCVBUF, 1024) })
			return err
		}}
		v, err := d.Dial("tcp", s.Addr)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := fmt.Fprintf(v, "NEWGROUPS 000101 000000 GMT\r\n"); err != nil {
			t.Fatal(err)
		}
		// Both timeouts are due after 2 seconds; with nothing to read and no room to write,
		// the reader looks at the descriptor in its check of all descriptors, every 30 to
		// 60 seconds.
		t0 := time.Now()
		testkit.WaitFor(t, 90*time.Second, "a timeout of the client that does not read", func() bool {
			return count("idle") > idle || count("session") > session
		})
		time.Sleep(time.Second)
		both = count("idle") == idle+1 && count("session") == session+1
		// the client did get part of the answer: it was ended in the middle of it
		v.SetReadDeadline(time.Now().Add(5 * time.Second))
		r := bufio.NewReader(v)
		got := 0
		for {
			l, err := r.ReadString('\n')
			if err != nil {
				break
			}
			if strings.HasPrefix(l, "e2e.lc") {
				got++
			}
		}
		v.Close()
		if got >= len(groups) {
			t.Fatalf("the scenario did not develop: the client that did not read got all %d groups", got)
		}
		t.Logf("try %d: idle and session timeout in one pass: %v (%d groups sent, after %v)", try, both, got, time.Since(t0).Round(time.Second))
	}
	if !both {
		t.Fatalf("the scenario did not develop: the idle and the session timeout never fired in the same pass")
	}

	const added = "e2e.lcnew"
	s.Run(t, "dgrpctl", "newgroup", added, "y")
	rescan := newgroups()
	if !slices.Contains(rescan, added) {
		t.Fatalf("NEWGROUPS after the new group: %s is not in it", added)
	}
	if got, want := newgroups(), append(slices.Clone(cached), added); !slices.Equal(got, want) {
		if slices.Equal(got, rescan) {
			t.Errorf("the cache of the group list was not built again after a connection ended by both timeouts at once: " +
				"NEWGROUPS keeps the order of the scan (the read lock count went below zero)")
		} else {
			t.Errorf("NEWGROUPS after the new group, the second time: %d groups, neither in the order of the cache nor of the scan", len(got))
		}
	}
}

package e2e

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"

	testkit "github.com/go-while/nntp-testkit"
)

// clockJumpSource moves the clock of gettimeofday() (which the reader's idle and session
// timeouts go by) forward by the number of seconds in the file CLOCKJUMP_FILE, read on every
// call, and marks each new shift it sees, once per process.
const clockJumpSource = `#define _GNU_SOURCE
#define __EXTENSIONS__
#include <dlfcn.h>
#include <fcntl.h>
#include <stdarg.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/select.h>
#include <unistd.h>
/*MARK*/
/*
 * <sys/time.h> is not included: its prototype differs between systems (struct timezone * or
 * void *). NetBSD's header renames gettimeofday() to __gettimeofday50 (today's struct
 * timeval); a program built there calls that name, and "gettimeofday" is the old one.
 */
#ifdef __NetBSD__
#define GTOD __gettimeofday50
#define GTOD_NAME "__gettimeofday50"
#else
#define GTOD gettimeofday
#define GTOD_NAME "gettimeofday"
#endif

int
GTOD(struct timeval *tv, void *tz)
{
    static int (*real)(struct timeval *, void *);
    static long seen;
    const char *f = getenv("CLOCKJUMP_FILE");
    long shift = 0;
    int fd;
    int r;

    if (real == NULL)
	real = (int (*)(struct timeval *, void *))dlsym(RTLD_NEXT, GTOD_NAME);
    if (real == NULL) {
	shim_mark("dlsym(%s) failed", GTOD_NAME);
	abort();
    }
    r = real(tv, tz);
    if (r != 0 || tv == NULL)
	return r;
    if (f != NULL && (fd = open(f, O_RDONLY)) >= 0) {
	char buf[32];
	ssize_t n = read(fd, buf, sizeof(buf) - 1);

	if (n > 0) {
	    buf[n] = 0;
	    shift = strtol(buf, NULL, 10);
	}
	close(fd);
    }
    tv->tv_sec += shift;
    if (shift != seen) {
	seen = shift;
	shim_mark("gettimeofday shifted by %ld", shift);
    }
    return r;
}
`

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
// apart. With idletimeout and sessiontimeout 600 (no timeout during the setup), a client
// sends NEWGROUPS and does not read, so it stays in the middle of its answer from the cache;
// then a preloaded gettimeofday() moves the reader's clock an hour on, past both limits, and
// the reader's next check of all descriptors ends the client by both timeouts in one pass.
// Then a new group: the second NEWGROUPS after it must come from the rebuilt cache again.
func TestDeepRevListCacheUnlockOnce(t *testing.T) {
	so := preloadShim(t, "clockjump", withMark(clockJumpSource))
	dir := t.TempDir()
	jump := filepath.Join(dir, "jump")
	mark := filepath.Join(dir, "mark")
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
    idletimeout 600
    sessiontimeout 600
    txbufsize 512
end
access 127.0.0.1 rp
`
	_, spool := spoolStore(t)
	s := NewReaderSite(t, ReaderOptions{Spool: spool, Groups: groups, Access: access, NoStart: true})
	s.Env = append(s.Env, lockFaultEnv(so), "CLOCKJUMP_FILE="+jump, "SHIM_MARK="+mark)
	t.Cleanup(func() {
		if t.Failed() {
			t.Logf("general.log:\n%s\nmarks:\n%s", generalLog(s), strings.Join(shimMarks(mark), "\n"))
		}
	})
	startReader(t, s, "[127.0.0.1]:"+strings.TrimPrefix(s.Addr, "127.0.0.1:"))

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

	// a client that asks and does not read: it stays in the middle of the answer from the cache
	count := func(what string) int { return strings.Count(generalLog(s), "timeout "+what+" 127.0.0.1") }
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
	defer v.Close()
	if _, err := fmt.Fprintf(v, "NEWGROUPS 000101 000000 GMT\r\n"); err != nil {
		t.Fatal(err)
	}
	time.Sleep(time.Second) // the reader has filled the buffers and waits to write

	// both limits are past at once
	if err := os.WriteFile(jump, []byte("3600"), 0o644); err != nil {
		t.Fatal(err)
	}
	// A busy connection makes the reader loop turn, so that it looks at its clock (every ten
	// passes) and then checks all descriptors; the timeouts end it too, as it started before the
	// jump.
	busy := s.Reader(t)
	testkit.WaitFor(t, 60*time.Second, "the timeouts of the client that does not read", func() bool {
		busy.TryCmd("DATE")
		return count("idle") > idle || count("session") > session
	})
	busy.Close()
	time.Sleep(time.Second)
	if len(shimMarked(mark, "gettimeofday shifted by 3600")) == 0 {
		t.Fatalf("no process saw the clock moved on through the injector")
	}
	if count("idle") != idle+1 || count("session") != session+1 {
		t.Fatalf("the scenario did not develop: the idle timeout fired %d and the session timeout %d times for the client, want both once in one pass",
			count("idle")-idle, count("session")-session)
	}
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
	if got >= len(groups) {
		t.Fatalf("the scenario did not develop: the client that did not read got all %d groups", got)
	}
	t.Logf("idle and session timeout in one pass after %d of %d groups", got, len(groups))

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

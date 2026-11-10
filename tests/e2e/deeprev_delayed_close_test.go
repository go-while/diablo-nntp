package e2e

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	testkit "github.com/go-while/nntp-testkit"
)

// timeShiftSource moves the clock of time() by the number of seconds in the file
// TIMESHIFT_FILE (none or no file: the real clock), read on every call, and marks each change
// of the shift it sees, once per process.
const timeShiftSource = `#define _GNU_SOURCE
#define __EXTENSIONS__
#include <dlfcn.h>
#include <fcntl.h>
#include <stdarg.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <time.h>
#include <unistd.h>
/*MARK*/
time_t
time(time_t *tp)
{
    static time_t (*real)(time_t *);
    static long seen;
    const char *f = getenv("TIMESHIFT_FILE");
    long shift = 0;
    time_t now;
    int fd;

    if (real == NULL)
	real = (time_t (*)(time_t *))dlsym(RTLD_NEXT, "time");
    if (f != NULL && (fd = open(f, O_RDONLY)) >= 0) {
	char buf[32];
	ssize_t n = read(fd, buf, sizeof(buf) - 1);

	if (n > 0) {
	    buf[n] = 0;
	    shift = strtol(buf, NULL, 10);
	}
	close(fd);
    }
    now = real(NULL) + shift;
    if (shift != seen) {
	seen = shift;
	shim_mark("time shifted by %ld", shift);
    }
    if (tp != NULL)
	*tp = now;
    return now;
}
`

// TestDeepRevDelayedCloseWalk: with -c <seconds> the main dreaderd process answers a refused
// client after that delay; the clients wait on a list in the order they came (main.c,
// NeedClosing), and every pass of the main loop closes the ones that are due. The walk kept
// a pointer to the entry it had just freed and unlinked the next due entry from that freed
// one. With the list in time order the due entries are a prefix and nothing went wrong; with
// an entry that is not due in front of two that are, the second stayed on the list after it
// was freed. The next pass wrote to and closed the descriptor in the freed entry (0, what the
// allocator left there) and freed it a second time, and the allocator stopped the main
// process ("corrupt memlist").
//
// The list gets out of order when the clock steps back. A preloaded time() steps it back
// by two minutes after the first refused client (A) is on the list: the next two (B, C) are
// due long before A. B and C must get their refusal and the close, A must wait, the main
// process must stay up, and A is answered once the clock is back.
func TestDeepRevDelayedCloseWalk(t *testing.T) {
	so := preloadShim(t, "timeshift", withMark(timeShiftSource))
	dir := t.TempDir()
	shift := filepath.Join(dir, "shift")
	mark := filepath.Join(dir, "mark")
	// 127.0.0.1 reads, PeerIP has no access line: it is refused
	access := `vserverdef DEFAULT
    hostname ` + PathHost + `
end
readerdef rp
    read yes
end
access 127.0.0.1 rp
`
	_, spool := spoolStore(t)
	s := NewReaderSite(t, ReaderOptions{Spool: spool, Access: access, NoStart: true})
	s.Env = append(s.Env, lockFaultEnv(so), "TIMESHIFT_FILE="+shift, "SHIM_MARK="+mark)
	t.Cleanup(func() {
		if t.Failed() {
			t.Logf("general.log:\n%s\nmarks:\n%s", generalLog(s), strings.Join(shimMarks(mark), "\n"))
		}
	})
	startReaderArgs(t, s, "-c", "3")
	if g := waitGreeting(s.Addr, testkit.Deadline()); !strings.HasPrefix(g, "20") {
		t.Fatalf("greeting %q, want 200 or 201", g)
	}
	mainPid := readMainPid(t, s)

	refused := func(n int) {
		t.Helper()
		testkit.WaitFor(t, 10*time.Second, fmt.Sprintf("%d refused clients in the log", n), func() bool {
			return strings.Count(generalLog(s), "rejected: Unauthorized") >= n
		})
	}
	dial := func() net.Conn {
		t.Helper()
		d := net.Dialer{LocalAddr: &net.TCPAddr{IP: net.ParseIP(PeerIP)}, Timeout: 10 * time.Second}
		c, err := d.Dial("tcp", s.Addr)
		if err != nil {
			t.Fatalf("dial from %s: %v", PeerIP, err)
		}
		t.Cleanup(func() { c.Close() })
		return c
	}
	setShift := func(sec int) {
		t.Helper()
		if err := os.WriteFile(shift, []byte(fmt.Sprint(sec)), 0o644); err != nil {
			t.Fatal(err)
		}
		want := fmt.Sprintf("%d time shifted by %d", mainPid, sec)
		testkit.WaitFor(t, 10*time.Second, "the main process to see the clock shifted by "+fmt.Sprint(sec), func() bool {
			return len(shimMarked(mark, want)) > 0
		})
	}
	refusal := func(name string, c net.Conn, d time.Duration) {
		t.Helper()
		text, eof := readToEOF(c, d)
		if !strings.HasPrefix(text, "502 ") || !eof {
			t.Errorf("client %s: got %q (closed: %v), want the 502 refusal and the close", name, text, eof)
		}
	}

	a := dial()
	refused(1)
	setShift(-120)
	b, c := dial(), dial()
	refused(3)

	refusal("B", b, 15*time.Second)
	refusal("C", c, 15*time.Second)
	// the scenario: A was not due while B and C were
	if text, eof := readToEOF(a, 500*time.Millisecond); text != "" || eof {
		t.Fatalf("the scenario did not develop: client A was answered (%q, closed: %v) with the clock two minutes back", text, eof)
	}
	// a few passes of the main loop over the list as the walk left it
	time.Sleep(3 * time.Second)
	if g := readerGreeting(s.Addr); !strings.HasPrefix(g, "20") {
		t.Errorf("after the delayed closes of B and C: greeting %q, want 200 or 201: the main process is gone", g)
	}
	if strings.Contains(generalLog(s), "corrupt memlist") {
		t.Errorf("the main process freed a delayed-close entry twice: \"corrupt memlist\" in the log")
	}

	setShift(0)
	refusal("A", a, 15*time.Second)
}

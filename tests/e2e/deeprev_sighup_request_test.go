package e2e

import (
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	testkit "github.com/go-while/nntp-testkit"
	"github.com/go-while/nntp-testkit/client"
	"github.com/go-while/nntp-testkit/fakeserver"
)

// TestDeepRevSighupRequestInFlight (L4-07): on SIGHUP ("drcmd exit") a reader process drops
// its clients. It freed a client that still had an article request out at a spool server
// without taking the client off the request, and it goes through all its connections in the
// order of their descriptor numbers. When the server's answer was already there and the
// server link had the higher number (a link that was set up again after the client came),
// the answer was then written into the freed client: a fault in the reader, which in a
// shutdown nobody sees, except through "readercrash".
//
// The spool server is started after the client is connected, so its link has the higher
// descriptor. The spool server holds back its answer to the client's request until the test
// lets it go. Once the reader has sent the request and waits again (in select(); on Linux
// seen as state S), it is stopped (SIGSTOP) and gets SIGHUP; then
// the answer is written, and only once the spool server's write has returned (on Linux also:
// once the answer waits in the receive queue of the reader's spool link) is the reader
// continued, so it finds the signal and the answer together. The client must be closed
// without the answer (the state above: freed while the answer was there), the reader must
// end, and the crash command must not run.
//
// A write into the freed connection did not always fault: the pool pages of the client are
// given back with munmap(), and the next mmap() (a buffer for the server's answer) often got
// the same address back, so the write went into live memory and the test passed (1 of 9 runs
// against the old code). Where an LD_PRELOAD injector can be used, munmap() makes the pages
// inaccessible instead (unmapTrapSource), so such a write faults. The injector marks each
// munmap() only after mprotect() has succeeded (a failure is marked as such), and marks the
// close() of the client's descriptor in the reader with the number of munmap() calls (pool
// areas) it made inaccessible since the close() before it: DeleteConnection() frees the client's pools (the Connection
// structure among them) right before DelThread() closes the descriptor, so a count above 0
// there ties the protected pages to the freed client. The test requires that mark after
// SIGCONT, and no unprotected munmap() in the reader. The run against the old code is not
// certain to fault on every schedule; it did in every run measured (see the commit).
func TestDeepRevSighupRequestInFlight(t *testing.T) {
	requireProcessInspection(t, false)
	port := testkit.FreePort(t)
	addr := "127.0.0.1:" + strconv.Itoa(port)
	s := NewReaderSite(t, ReaderOptions{SpoolAddr: addr, NoStart: true,
		Config: map[string]string{"readercrash": "@home@/crash.sh"}})
	mark, clientFile := "", ""
	if preloadHost(true) {
		so := preloadShim(t, "unmaptrap", withMark(unmapTrapSource))
		dir := t.TempDir()
		mark, clientFile = filepath.Join(dir, "mark"), filepath.Join(dir, "client")
		s.Env = append(s.Env, lockFaultEnv(so), "SHIM_MARK="+mark, "UNMAPTRAP_CLIENT="+clientFile)
	} else {
		t.Logf("no LD_PRELOAD on %s: a write into the freed client may go unnoticed", runtime.GOOS)
	}
	crashLog := filepath.Join(s.Dir, "crash.log")
	s.WriteFile(t, "crash.sh", "#!/bin/sh\necho \"$1\" >> "+crashLog+"\n")
	if err := os.Chmod(filepath.Join(s.Dir, "crash.sh"), 0o755); err != nil {
		t.Fatal(err)
	}
	_, sitePort, _ := strings.Cut(s.Addr, ":")
	startReader(t, s, "[127.0.0.1]:"+sitePort)
	deepRevKillChildren(t, s, "dreaderd")

	st := fakeserver.NewStore()
	st.AddGroup("e2e.test", "e2e test group")
	id := msgID(t)
	art := newArticle(id, "e2e.test", "sighup", "the body")
	feedReader(t, s, st, "e2e.test", art)

	// The client first: it gets a lower descriptor in the reader than the spool link.
	c := s.Reader(t)
	if clientFile != "" {
		_, cport, _ := net.SplitHostPort(c.Conn.LocalAddr().String())
		if err := os.WriteFile(clientFile, []byte(cport+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	testkit.WaitFor(t, testkit.Deadline(), "the article in GROUP e2e.test", func() bool {
		_, line := c.Cmd("GROUP e2e.test")
		f := strings.Fields(line)
		return len(f) > 1 && f[0] == "211" && f[1] != "0"
	})

	// The spool server: the first request for the article waits for release, then the
	// answer is written and answered gets the result of that write.
	requested, release := make(chan struct{}), make(chan struct{})
	answered := make(chan error, 1)
	var first sync.Once
	reply := func(code int, lines []string) []byte {
		return []byte(strconv.Itoa(code) + " 0 " + id + "\r\n" + client.Stuff(lines))
	}
	_, body := splitArticle(art)
	spool := fakeserver.New(t, fakeserver.Options{Addr: addr, Handler: fakeserver.LineHandler(func(sc *fakeserver.Conn, line string) ([]string, bool) {
		f := strings.Fields(line)
		if len(f) == 0 {
			return []string{"500 What?"}, false
		}
		switch strings.ToUpper(f[0]) {
		case "MODE":
			return []string{"200 fake spool"}, false
		case "QUIT":
			return []string{"205 bye"}, true
		case "ARTICLE", "BODY":
			if len(f) < 2 || f[1] != id {
				return []string{"430 no such article"}, false
			}
		default:
			return []string{"500 What?"}, false
		}
		var a []byte
		if strings.EqualFold(f[0], "BODY") {
			a = reply(222, body)
		} else {
			a = reply(220, art)
		}
		held := false
		first.Do(func() { held = true; close(requested) })
		if !held {
			if _, err := sc.Raw().Write(a); err != nil {
				return nil, true
			}
			return nil, false
		}
		if !sc.Wait(release) {
			answered <- errors.New("the answer was never released")
			return nil, true
		}
		_, err := sc.Raw().Write(a)
		answered <- err
		return nil, err != nil
	})})
	// Commands keep the reader's loop turning until it has tried the spool server again.
	limit := time.Now().Add(90 * time.Second)
	for spool.Open() == 0 {
		if time.Now().After(limit) {
			t.Fatalf("the reader did not connect to the spool server in 90 s\n%s", s.logOutput())
		}
		c.ExpectCmd(111, "DATE")
		time.Sleep(200 * time.Millisecond)
	}
	time.Sleep(2 * time.Second) // the link is set up (MODE READER and so on)

	var reader int
	for p := range readerForkPids(s) {
		reader, _ = strconv.Atoi(p)
	}
	if reader == 0 {
		t.Fatalf("no reader process in the output of dreaderd:\n%s", s.logOutput())
	}

	if err := c.Send("BODY " + id + "\r\n"); err != nil {
		t.Fatal(err)
	}
	select {
	case <-requested:
	case <-time.After(10 * time.Second):
		t.Fatalf("the reader sent no request for %s to the spool server in 10 s; it got: %q", id, spool.Lines())
	}
	// The request is out and its answer held back. The reader first ends the pass in which it
	// sent the request and waits in select(): stopped in the middle of that pass, it would
	// read the answer on SIGCONT before it looks at the signal and hand it to the client.
	// Then stop the reader, signal it, and only then let the answer go.
	if known := waitProcessSleeping(reader, 300*time.Millisecond, 10*time.Second); known != nil && !*known {
		t.Fatalf("the reader %d did not wait (state S) within 10 s of its request", reader)
	}
	if err := syscall.Kill(reader, syscall.SIGSTOP); err != nil {
		t.Fatalf("SIGSTOP to the reader %d: %v", reader, err)
	}
	if known := waitProcessState(reader, "Tt", 0, 5*time.Second); known != nil && !*known {
		t.Fatalf("the reader %d did not stop within 5 s of SIGSTOP", reader)
	}
	_ = syscall.Kill(reader, syscall.SIGHUP)
	close(release)
	select {
	case err := <-answered:
		if err != nil {
			t.Fatalf("the spool server could not write its answer: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatalf("the spool server did not write its answer within 10 s of the release")
	}
	if queued, known := spoolAnswerQueued(reader, port, 5*time.Second); known && !queued {
		t.Fatalf("the answer of the spool server is not in the receive queue of the stopped reader's link 5 s after it was written")
	} else if known {
		t.Logf("the answer waits in the receive queue of the stopped reader's spool link")
	}
	protectedBefore := unmapMarks(mark, reader, " client close ")
	_ = syscall.Kill(reader, syscall.SIGCONT)

	// The client is dropped before the answer reaches it.
	lines, err := c.ExpectClosed(10 * time.Second)
	for _, l := range lines {
		if strings.HasPrefix(l, "222") {
			t.Fatalf("the client got the answer (%q) before the reader dropped it: the test did not reach the state it is about (the answer there while the client is freed)", lines)
		}
	}
	if err != nil {
		t.Fatalf("the client was not closed after SIGHUP to its reader: %v", err)
	}

	// The reader ends (it has no client left); a fault would run the crash command.
	gone := false
	limit = time.Now().Add(10 * time.Second)
	for time.Now().Before(limit) {
		if b, _ := os.ReadFile(crashLog); len(b) > 0 {
			t.Fatalf("the reader process faulted after SIGHUP with a request in flight (the crash command ran): the answer of the spool server was written into the client connection it had just freed\n%s",
				generalLog(s))
		}
		if syscall.Kill(reader, 0) != nil {
			gone = true
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	time.Sleep(500 * time.Millisecond)
	if b, _ := os.ReadFile(crashLog); len(b) > 0 {
		t.Fatalf("the reader process faulted after SIGHUP with a request in flight (the crash command ran)\n%s", generalLog(s))
	}
	if !gone {
		t.Errorf("the reader %d still runs 10 s after SIGHUP: it did not drop its client", reader)
	}
	if mark == "" {
		return
	}
	if len(protectedBefore) > 0 {
		t.Errorf("the reader %d closed the client before SIGCONT: %q", reader, protectedBefore)
	}
	if bad := unmapMarks(mark, reader, " not protected "); len(bad) > 0 {
		t.Errorf("the injector could not make freed pages of the reader %d inaccessible (a write into them would go unnoticed): %q", reader, bad)
	}
	closes := unmapMarks(mark, reader, " client close ")
	t.Logf("injector, reader %d: %q", reader, closes)
	if len(closes) != 1 {
		t.Errorf("the reader %d closed the client %d times as marked by the injector, want once: %q", reader, len(closes), closes)
	} else if f := strings.Fields(closes[0]); len(f) < 6 || f[4] == "0" { // "<pid> client close after <n> protected"
		t.Errorf("the reader %d made no memory inaccessible between freeing the client and closing its descriptor (%q): the client's memory was not given back, or the injector did not act", reader, closes[0])
	}
}

// unmapTrapSource makes munmap() leave the pages mapped but inaccessible (PROT_NONE), so a
// later use of freed pool memory faults instead of reaching whatever the next mmap() put
// there. Each call is marked after mprotect() succeeded ("munmap <len> protected"); when it
// fails, the mark says "not protected" and the pages are unmapped for real. close() of the
// descriptor whose peer port is in the file UNMAPTRAP_CLIENT is marked "client close after
// <n> protected": the munmap() calls made inaccessible since the close() before it in the
// process.
// The address space is not given back: for short tests only.
const unmapTrapSource = `#define _GNU_SOURCE
#define __EXTENSIONS__
#include <dlfcn.h>
#include <errno.h>
#include <fcntl.h>
#include <stdarg.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/types.h>
#include <sys/mman.h>
#include <sys/socket.h>
#include <netinet/in.h>
#include <unistd.h>
/*MARK*/
static int Protected;	/* munmap() calls made PROT_NONE since the last close() */
static int InShim;	/* in the shim's own open(), read() and close() */

int
munmap(void *addr, size_t len)
{
    static int (*real)(void *, size_t);
    int e = errno;

    if (mprotect(addr, len, PROT_NONE) == 0) {
	++Protected;
	++InShim;
	shim_mark("munmap %lu protected", (unsigned long)len);
	--InShim;
	errno = e;
	return 0;
    }
    e = errno;
    ++InShim;
    shim_mark("munmap %lu not protected (errno %d)", (unsigned long)len, e);
    --InShim;
    if (real == NULL)
	real = (int (*)(void *, size_t))dlsym(RTLD_NEXT, "munmap");
    if (real == NULL) {
	errno = e;
	return -1;
    }
    return real(addr, len);
}

/* the client's port from the file UNMAPTRAP_CLIENT, 0 while there is none */
static int
client_port(void)
{
    const char *p = getenv("UNMAPTRAP_CLIENT");
    char buf[32];
    int fd;
    int n = -1;

    if (p == NULL)
	return 0;
    ++InShim;
    if ((fd = open(p, O_RDONLY)) >= 0) {
	n = read(fd, buf, sizeof(buf) - 1);
	close(fd);
    }
    --InShim;
    if (n <= 0)
	return 0;
    buf[n] = 0;
    return atoi(buf);
}

int
close(int fd)
{
    static int (*real)(int);
    struct sockaddr_in sin;
    socklen_t sl = sizeof(sin);
    int port;
    int e = errno;

    if (real == NULL)
	real = (int (*)(int))dlsym(RTLD_NEXT, "close");
    if (InShim)
	return real(fd);
    memset(&sin, 0, sizeof(sin));
    if (getpeername(fd, (struct sockaddr *)&sin, &sl) == 0 && sin.sin_family == AF_INET &&
	(port = client_port()) > 0 && ntohs(sin.sin_port) == port) {
	++InShim;
	shim_mark("client close after %d protected", Protected);
	--InShim;
    }
    Protected = 0;
    errno = e;	/* the caller sees only what close() itself does to it */
    return real(fd);
}
`

// unmapMarks returns the marks of unmapTrapSource for process pid that contain what (none
// without a mark file).
func unmapMarks(mark string, pid int, what string) []string {
	if mark == "" {
		return nil
	}
	var m []string
	for _, l := range shimMarked(mark, what) {
		if strings.HasPrefix(l, strconv.Itoa(pid)+" ") {
			m = append(m, l)
		}
	}
	return m
}

// waitProcessSleeping waits settle, then up to d until process pid sleeps (state S) on two
// looks 50 ms apart. Where the state cannot be read it only waits settle; see
// waitProcessState for the result.
func waitProcessSleeping(pid int, settle, d time.Duration) *bool {
	time.Sleep(settle)
	return waitProcessState(pid, "S", 50*time.Millisecond, d)
}

// waitProcessState waits up to d for process pid to be in one of the states in states (the
// letters of /proc/<pid>/stat), and when again is not 0, still in one of them again later.
// It returns nil where the state cannot be read (it is read from /proc on Linux only), else
// whether it got there.
func waitProcessState(pid int, states string, again, d time.Duration) *bool {
	if runtime.GOOS != "linux" {
		return nil
	}
	in := func() (bool, bool) {
		b, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
		if err != nil {
			return false, false
		}
		// pid (comm) state ...: comm may hold blanks, the state follows the last ')'
		i := strings.LastIndexByte(string(b), ')')
		return i >= 0 && i+2 < len(b) && strings.IndexByte(states, b[i+2]) >= 0, true
	}
	ok := false
	for limit := time.Now().Add(d); ; {
		yes, alive := in()
		if !alive {
			return &ok
		}
		if yes && again > 0 {
			time.Sleep(again)
			yes, alive = in()
			if !alive {
				return &ok
			}
		}
		if yes {
			ok = true
			return &ok
		}
		if time.Now().After(limit) {
			return &ok
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// spoolAnswerQueued waits up to d for unread bytes in the receive queue of an established TCP
// connection to 127.0.0.1:port, read from /proc/<pid>/net/tcp (every TCP socket of the
// network namespace of process pid; the spool links of other readers have nothing unread, so
// the one with bytes is the link that got the answer). known is false where that cannot be
// read (not Linux).
func spoolAnswerQueued(pid, port int, d time.Duration) (queued, known bool) {
	if runtime.GOOS != "linux" {
		return false, false
	}
	remote := fmt.Sprintf("0100007F:%04X", port)
	for limit := time.Now().Add(d); ; {
		b, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/net/tcp")
		if err != nil {
			return false, false
		}
		for _, l := range strings.Split(string(b), "\n")[1:] {
			// sl local_address rem_address st tx_queue:rx_queue ...
			f := strings.Fields(l)
			if len(f) < 5 || f[2] != remote || f[3] != "01" {
				continue
			}
			if _, rx, ok := strings.Cut(f[4], ":"); ok {
				if n, err := strconv.ParseUint(rx, 16, 64); err == nil && n > 0 {
					return true, true
				}
			}
		}
		if time.Now().After(limit) {
			return false, true
		}
		time.Sleep(10 * time.Millisecond)
	}
}

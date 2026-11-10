package e2e

import (
	"net"
	"path/filepath"
	"strings"
	"testing"
	"time"

	testkit "github.com/go-while/nntp-testkit"
)

// highFdSource moves descriptors to the number FDHIGH_AT. With FDHIGH_RECV set, the first
// descriptor a child of the main process (a reader) receives with recvmsg() is moved there;
// with FDHIGH_PAIR set, the first descriptor of the first socketpair() of the process that
// loaded the injector (the main process) is. The soft descriptor limit is raised for it when
// needed. Each move, or why there was none, is marked.
const highFdSource = `#define _GNU_SOURCE
#define __EXTENSIONS__
#include <dlfcn.h>
#include <errno.h>
#include <fcntl.h>
#include <stdarg.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/resource.h>
#include <sys/socket.h>
#include <sys/types.h>
#include <unistd.h>
/*MARK*/
static pid_t firstpid;

__attribute__((constructor)) static void
highfd_init(void)
{
    firstpid = getpid();
}

static int
highfd_move(int fd)
{
    const char *at = getenv("FDHIGH_AT");
    struct rlimit rl;
    int to;

    if (at == NULL || (to = atoi(at)) <= fd)
	return fd;
    if (getrlimit(RLIMIT_NOFILE, &rl) == 0 && rl.rlim_cur <= (rlim_t)to) {
	rl.rlim_cur = to + 1;
	if (rl.rlim_max != RLIM_INFINITY && rl.rlim_max < rl.rlim_cur) {
	    shim_mark("cannot move %d to %d: hard limit %ld", fd, to, (long)rl.rlim_max);
	    return fd;
	}
	(void)setrlimit(RLIMIT_NOFILE, &rl);
    }
    if (dup2(fd, to) != to) {
	shim_mark("cannot move %d to %d: %s", fd, to, strerror(errno));
	return fd;
    }
    close(fd);
    shim_mark("moved %d to %d", fd, to);
    return to;
}

ssize_t
recvmsg(int s, struct msghdr *msg, int flags)
{
    static ssize_t (*real)(int, struct msghdr *, int);
    static int done;
    struct cmsghdr *cmsg;
    ssize_t r;

    if (real == NULL)
	real = (ssize_t (*)(int, struct msghdr *, int))dlsym(RTLD_NEXT, "recvmsg");
    r = real(s, msg, flags);
    if (r <= 0 || done || getenv("FDHIGH_RECV") == NULL || getpid() == firstpid)
	return r;
    for (cmsg = CMSG_FIRSTHDR(msg); cmsg != NULL; cmsg = CMSG_NXTHDR(msg, cmsg)) {
	if (cmsg->cmsg_level == SOL_SOCKET && cmsg->cmsg_type == SCM_RIGHTS &&
	    cmsg->cmsg_len >= CMSG_LEN(sizeof(int))) {
	    int fd;

	    memcpy(&fd, CMSG_DATA(cmsg), sizeof(fd));
	    done = 1;
	    fd = highfd_move(fd);
	    memcpy(CMSG_DATA(cmsg), &fd, sizeof(fd));
	    break;
	}
    }
    return r;
}

int
socketpair(int domain, int type, int protocol, int sv[2])
{
    static int (*real)(int, int, int, int *);
    static int done;
    int r;

    if (real == NULL)
	real = (int (*)(int, int, int, int *))dlsym(RTLD_NEXT, "socketpair");
    r = real(domain, type, protocol, sv);
    if (r == 0 && !done && getenv("FDHIGH_PAIR") != NULL && getpid() == firstpid) {
	done = 1;
	sv[0] = highfd_move(sv[0]);
    }
    return r;
}
`

// TestDeepRevReaderFdTooLarge: a reader process that receives a client descriptor at or above
// MAXFDS (its select() sets end there) hands the connection back to the main process, which
// closes its copy and counts the connection as ended (dreaderd/reader.c, HandleReaderMsg()).
// The reader kept its own copy open: the client got no greeting and no close, and the reader
// lost one descriptor per such client.
//
// MAXFDS is the size of the select() set (1024 with glibc, up to 3960 elsewhere), so a reader
// needs that many descriptors open to get there. A preloaded recvmsg() moves the descriptor
// of the first client the reader receives to 4000 instead. The client must see the
// connection closed.
func TestDeepRevReaderFdTooLarge(t *testing.T) {
	so := preloadShim(t, "highfd", withMark(highFdSource))
	mark := filepath.Join(t.TempDir(), "mark")
	_, spool := spoolStore(t)
	s := NewReaderSite(t, ReaderOptions{Spool: spool, NoStart: true})
	s.Env = append(s.Env, lockFaultEnv(so), "SHIM_MARK="+mark, "FDHIGH_RECV=1", "FDHIGH_AT=4000")
	t.Cleanup(func() {
		if t.Failed() {
			t.Logf("general.log:\n%s\nmarks:\n%s", generalLog(s), strings.Join(shimMarks(mark), "\n"))
		}
	})
	startReaderArgs(t, s)
	testkit.WaitFor(t, testkit.Deadline(), "dreaderd to listen", func() bool {
		c, err := net.DialTimeout("tcp", s.Addr, time.Second)
		if err != nil {
			return false
		}
		// this first client is the one whose descriptor is moved
		defer c.Close()
		text, eof := readToEOF(c, 10*time.Second)
		if len(shimMarked(mark, "to 4000")) == 0 {
			t.Fatalf("the reader did not receive the client's descriptor through the injector (got %q)", text)
		}
		if m := shimMarked(mark, "cannot move"); len(m) > 0 {
			t.Skipf("the descriptor could not be moved to 4000: %s", m[0])
		}
		if !strings.Contains(generalLog(s), "fd too large 4000/") {
			t.Fatalf("the scenario did not develop: no \"fd too large 4000/...\" in the log")
		}
		if !eof {
			t.Fatalf("the client whose descriptor was too large for the reader got %q and no close within 10 s: the reader kept its copy open", text)
		}
		if text != "" {
			t.Errorf("the client whose descriptor was too large for the reader got %q, want only the close", text)
		}
		return true
	})
	// the next client is served as usual
	if g := readerGreeting(s.Addr); !strings.HasPrefix(g, "200") {
		t.Errorf("the next client: greeting %q, want 200", g)
	}
}

package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// notifyNonblockFailSource makes fcntl(F_SETFL) with O_NONBLOCK fail on Unix datagram sockets
// (the feed notify socket of diablo) and appends a line to the file NOTIFYNB_MARK when it
// does. The wrapper takes the third argument by the type the command has: none, an int or a
// pointer.
const notifyNonblockFailSource = `#define _GNU_SOURCE
#define __EXTENSIONS__
#include <dlfcn.h>
#include <errno.h>
#include <fcntl.h>
#include <stdarg.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>
#include <sys/types.h>
#include <sys/socket.h>
#include <sys/un.h>

typedef int (*fcntl_fn)(int, int, ...);

static int
unixdgram(int fd)
{
    struct sockaddr_storage ss;
    socklen_t sl = sizeof(ss);
    int type;
    socklen_t tl = sizeof(type);

    if (getsockopt(fd, SOL_SOCKET, SO_TYPE, &type, &tl) != 0 || type != SOCK_DGRAM)
	return 0;
    if (getsockname(fd, (struct sockaddr *)&ss, &sl) != 0)
	return 0;
    return ss.ss_family == AF_UNIX;
}

static void
note(void)
{
    const char *f = getenv("NOTIFYNB_MARK");
    int mfd;

    if (f != NULL && (mfd = open(f, O_WRONLY|O_CREAT|O_APPEND, 0644)) >= 0) {
	(void)write(mfd, "F_SETFL\n", 8);
	close(mfd);
    }
}

/* commands whose third argument is an int; the others here take none or a pointer */
static int
intarg(int cmd)
{
    switch (cmd) {
    case F_DUPFD:
#ifdef F_DUPFD_CLOEXEC
    case F_DUPFD_CLOEXEC:
#endif
    case F_SETFD:
    case F_SETFL:
#ifdef F_SETOWN
    case F_SETOWN:
#endif
	return 1;
    }
    return 0;
}

static int
noarg(int cmd)
{
    switch (cmd) {
    case F_GETFD:
    case F_GETFL:
#ifdef F_GETOWN
    case F_GETOWN:
#endif
	return 1;
    }
    return 0;
}

static int
wrap(const char *name, int fd, int cmd, va_list va)
{
    fcntl_fn real = (fcntl_fn)dlsym(RTLD_NEXT, name);

    if (real == NULL)
	real = (fcntl_fn)dlsym(RTLD_NEXT, "fcntl");
    if (noarg(cmd))
	return real(fd, cmd);
    if (intarg(cmd)) {
	int arg = va_arg(va, int);

	if (cmd == F_SETFL && (arg & O_NONBLOCK) && unixdgram(fd)) {
	    note();
	    errno = EINVAL;
	    return -1;
	}
	return real(fd, cmd, arg);
    }
    return real(fd, cmd, va_arg(va, void *));
}

int
fcntl(int fd, int cmd, ...)
{
    va_list va;
    int r;

    va_start(va, cmd);
    r = wrap("fcntl", fd, cmd, va);
    va_end(va);
    return r;
}

#ifdef __linux__
int
fcntl64(int fd, int cmd, ...)
{
    va_list va;
    int r;

    va_start(va, cmd);
    r = wrap("fcntl64", fd, cmd, va);
    va_end(va);
    return r;
}
#endif
`

// TestDeepRevNotifyLogNamesFeed: when diablo cannot make a feed notify socket non-blocking,
// it logs that and gives up on the socket. The message printed the socket path, which is
// filled in only after that point: it printed whatever was on the stack. It names the feed
// now.
func TestDeepRevNotifyLogNamesFeed(t *testing.T) {
	so := preloadShim(t, "notifynb", notifyNonblockFailSource)
	mark := filepath.Join(t.TempDir(), "setfl")
	s := NewSite(t, SiteOptions{Newsfeeds: OutNewsfeeds, Env: []string{lockFaultEnv(so), "NOTIFYNB_MARK=" + mark}})
	s.Inject(t, "e2e.test", 1)
	// feednotify acts on a feed that has queue lines in diablo's buffer: ask until it did
	const msg = "Unable to set notify socket as non-blocking"
	limit := time.Now().Add(10 * time.Second)
	for !strings.Contains(generalLog(s), msg) {
		if time.Now().After(limit) {
			t.Fatalf("the scenario did not develop: diablo did not log %q", msg)
		}
		// diablo sends no reply to feednotify, so dicmd exits 1
		s.TryRun("dicmd", "feednotify", "on:"+OutLabel)
		time.Sleep(100 * time.Millisecond)
	}
	if b, err := os.ReadFile(mark); err != nil || len(b) == 0 {
		t.Fatalf("the fault injector did not fail F_SETFL on the notify socket (%v)", err)
	}
	for _, l := range strings.Split(generalLog(s), "\n") {
		if strings.Contains(l, msg) && !strings.Contains(l, msg+": "+OutLabel+" (") {
			t.Errorf("the message does not name the feed %q: %q", OutLabel, l)
		}
	}
}

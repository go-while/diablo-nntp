package e2e

import (
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestReaderHandbackFdLostReleased: when the descriptor a reader fork hands back to the main
// dreaderd process is lost on the way (the kernel drops an SCM_RIGHTS descriptor and sets
// MSG_CTRUNC when the receiver is at its descriptor limit), the main process still releases
// the connection: "drcmd status" goes back to Act=0 and the next login works. A preloaded
// shim does to the main process's recvmsg() what Linux does at the limit, while the file
// FDLOST_FLAG exists. Fails before #266: RecvMsg() returned -1 on MSG_CTRUNC, the DnsRes was
// thrown away and the connection stayed counted (Act=1) until a restart.
//
// Two hand-backs: a session that ends (QUIT), and an AUTHINFO login, whose descriptor goes
// to the main process for the authentication. A third case does what FreeBSD does at the
// limit (FDLOST_MODE=emfile): recvmsg() fails once with EMFILE and leaves the message
// queued, and the next recvmsg() gets the data without the descriptor and without
// MSG_CTRUNC. The main process logs the error and still releases the connection.
func TestReaderHandbackFdLostReleased(t *testing.T) {
	so := preloadShim(t, "fdlost", fdLostSource)
	for _, tc := range []struct {
		name, access, mode string
	}{
		{"quit", "", ""},
		{"authinfo", AuthReaderAccess, ""},
		{"emfile then data", "", "emfile"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			flag := filepath.Join(t.TempDir(), "lose")
			_, spool := spoolStore(t)
			// The shim comes in dreaderd's environment (Env, not NoStart and startReader): the
			// site's listening socket is handed over as it was opened, not closed and bound again
			// (that bind failed with "address already in use" once, on illumos).
			s := NewReaderSite(t, ReaderOptions{Spool: spool, Access: tc.access,
				Env:    []string{lockFaultEnv(so), "FDLOST_FLAG=" + flag, "FDLOST_MODE=" + tc.mode},
				Config: map[string]string{"path_generallog": "%s/general.log"}})

			act0 := func(what string) {
				t.Helper()
				var st string
				limit := time.Now().Add(15 * time.Second)
				for {
					st = s.Run(t, "drcmd", "status")
					if strings.Contains(st, "Act=0/") || time.Now().After(limit) {
						break
					}
					time.Sleep(200 * time.Millisecond)
				}
				if !strings.Contains(st, "Act=0/") {
					t.Errorf("drcmd status %s: %q, want Act=0", what, strings.TrimSpace(st))
				}
			}

			if err := os.WriteFile(flag, nil, 0o644); err != nil {
				t.Fatal(err)
			}
			c := s.Reader(t)
			if tc.access == "" {
				c.ExpectCmd(205, "QUIT")
			} else {
				c.ExpectCmd(381, "AUTHINFO USER fred")
				// The descriptor is lost on its way to the authentication: the client
				// gets no answer, only the end of the connection.
				code, line, err := c.TryCmd("AUTHINFO PASS fredspass")
				if err == nil {
					t.Errorf("AUTHINFO PASS with the descriptor lost: %d %q, want the end of the connection", code, line)
				}
			}
			c.Close()
			act0("after the descriptor was lost")
			if tc.mode == "emfile" {
				// illumos syslog has no LOG_PERROR: the line is in the log file there.
				// The EMFILE text differs by libc (musl: "No file descriptors available").
				logged, _ := os.ReadFile(filepath.Join(s.Dir, "log", "general.log"))
				if !strings.Contains(string(logged)+s.Proc.Stderr(), "recvmsg error (") {
					t.Errorf("no \"recvmsg error\" in log/general.log or stderr:\n%s", logged)
				}
			}

			if err := os.Remove(flag); err != nil {
				t.Fatal(err)
			}
			c = s.Reader(t)
			if tc.access != "" {
				c.ExpectCmd(381, "AUTHINFO USER fred")
				c.ExpectCmd(281, "AUTHINFO PASS fredspass")
			}
			c.ExpectCmd(211, "GROUP e2e.test")
			c.ExpectCmd(205, "QUIT")
			act0("after the next session")
		})
	}
}

// TestReaderHandbackFullTableKept: D-170. When the main dreaderd process is at its
// descriptor limit as a reader hands a connection back, illumos fails recvmsg() with EMFILE
// and throws the whole message away, so the main process never learns that the connection
// ended and it stays counted (Act=1) until a restart. The shim (FDLOST_MODE=full) fills the
// main process's descriptor table at the select() after FDLOST_FLAG appears, and its recvmsg()
// does what illumos does when no slot is free (on every system, so the case runs everywhere).
// The main process keeps a spare descriptor and closes it just before each hand-back, so a
// slot is free, the descriptor arrives, and nothing is logged as lost.
// Fails before D-170: the message is thrown away and Act stays 1.
func TestReaderHandbackFullTableKept(t *testing.T) {
	so := preloadShim(t, "fdlost", fdLostSource)
	flag := filepath.Join(t.TempDir(), "full")
	fdlog := filepath.Join(t.TempDir(), "fdlost.log")
	_, spool := spoolStore(t)
	s := NewReaderSite(t, ReaderOptions{Spool: spool, NoStart: true,
		Config: map[string]string{"path_generallog": "%s/general.log"}})
	s.Env = append(s.Env, lockFaultEnv(so), "FDLOST_FLAG="+flag, "FDLOST_MODE=full", "FDLOST_LOG="+fdlog)
	_, port, _ := net.SplitHostPort(s.Addr)
	startReader(t, s, "[127.0.0.1]:"+port)

	act0 := func(what string) {
		t.Helper()
		var st string
		limit := time.Now().Add(15 * time.Second)
		for {
			st = s.Run(t, "drcmd", "status")
			if strings.Contains(st, "Act=0/") || time.Now().After(limit) {
				break
			}
			time.Sleep(200 * time.Millisecond)
		}
		if !strings.Contains(st, "Act=0/") {
			t.Errorf("drcmd status %s: %q, want Act=0", what, strings.TrimSpace(st))
		}
	}

	c := s.Reader(t)
	c.ExpectCmd(211, "GROUP e2e.test")
	// The table fills at the main process's next select(): the one that sees the hand-back.
	if err := os.WriteFile(flag, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	c.ExpectCmd(205, "QUIT")
	c.Close()
	act0("after a hand-back at the descriptor limit")
	logged, _ := os.ReadFile(filepath.Join(s.Dir, "log", "general.log"))
	// The shim writes its lines to FDLOST_LOG before it returns from select() or recvmsg(),
	// so before dreaderd goes on with the hand-back and before act0 can see Act=0. What it
	// prints to stderr only reaches s.Proc.Stderr() when the copy goroutine has read it
	// from the pipe, which can come after this point.
	shim, _ := os.ReadFile(fdlog)
	all := string(logged) + s.Proc.Stderr() + string(shim)
	if !strings.Contains(string(shim), "fdlost: table full") {
		t.Errorf("the shim never filled the descriptor table (no \"fdlost: table full\" in %s):\n%s", fdlog, all)
	}
	for _, bad := range []string{"recvmsg error (", "recvmsg descriptor dropped", "fdlost: message lost"} {
		if strings.Contains(all, bad) {
			t.Errorf("%q logged: the hand-back had no free slot:\n%s", bad, all)
		}
	}
	if err := os.Remove(flag); err != nil {
		t.Fatal(err)
	}

	c = s.Reader(t)
	c.ExpectCmd(211, "GROUP e2e.test")
	c.ExpectCmd(205, "QUIT")
	act0("after the next session")
}

// fdLostSource is the shim: in the process that loaded it first (the main dreaderd
// process; the reader forks inherit the recorded pid and are left alone), while the file
// FDLOST_FLAG exists, a descriptor received with recvmsg() is closed and reported as lost
// (no control message, MSG_CTRUNC; Linux). With FDLOST_MODE=emfile a message that carries a
// descriptor first fails with EMFILE and stays queued, and the next recvmsg() returns it
// without the descriptor and without MSG_CTRUNC (FreeBSD). With FDLOST_MODE=full the first
// select() that returns while the flag exists fills the descriptor table (dup() until it
// fails; "fdlost: table full" on stderr and in FDLOST_LOG), and the next recvmsg() empties it again: if no slot
// was free when it was called and the message carries a descriptor, the message is read and
// thrown away with its descriptor, and recvmsg() fails with EMFILE ("fdlost: message lost";
// illumos). A message without a descriptor (reader stats) passes and the table fills again at
// the next select(). If no recvmsg() comes, the table is emptied at a select() 3 seconds later.
// If FDLOST_LOG names a file, the shim also appends its "fdlost: table full" and "fdlost:
// message lost" lines there (written before it returns), so that a test can read them in
// order with what dreaderd does.
const fdLostSource = `#if defined(__sun)
#define _XOPEN_SOURCE 600	/* as diablo is built: recvmsg() is __xnet_recvmsg() */
#define __EXTENSIONS__
#define REAL_RECVMSG "__xnet_recvmsg"
#else
#define _GNU_SOURCE
#define REAL_RECVMSG "recvmsg"
#endif
#if defined(__NetBSD__)
#define REAL_SELECT "__select50"	/* <sys/select.h> renames select() to it */
#else
#define REAL_SELECT "select"
#endif
#include <sys/types.h>
#include <sys/socket.h>
#include <sys/select.h>
#include <sys/time.h>
#include <dlfcn.h>
#include <fcntl.h>
#include <stdio.h>
#include <time.h>
#include <errno.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>

static pid_t mainPid;
static int pending;	/* emfile: the next message is returned without its descriptor */
static int *fill;	/* full: the descriptors that fill the table */
static int nfill, filled;
static time_t filledAt;
static int logfd = -1;	/* full: FDLOST_LOG, opened before the table is filled */

static int
fullmode(void)
{
    const char *flag = getenv("FDLOST_FLAG");
    const char *mode = getenv("FDLOST_MODE");

    return(getpid() == mainPid && flag && mode && strcmp(mode, "full") == 0 &&
	access(flag, F_OK) == 0);
}

static void
unfill(void)
{
    while (nfill > 0)
	close(fill[--nfill]);
    if (logfd >= 0) {
	close(logfd);
	logfd = -1;
    }
}

/* appends line to the file FDLOST_LOG names (if set), on fd if that is open already: the
   test reads it instead of the copy of stderr, which can lag behind */
static void
note(int fd, const char *line)
{
    const char *lf = getenv("FDLOST_LOG");
    int own = 0;

    if (fd < 0) {
	if (lf == NULL || (fd = open(lf, O_WRONLY | O_APPEND | O_CREAT, 0644)) < 0)
	    return;
	own = 1;
    }
    if (write(fd, line, strlen(line)) < 0)
	fprintf(stderr, "fdlost: cannot write the log\n");
    if (own)
	close(fd);
}

int
select(int n, fd_set *r, fd_set *w, fd_set *e, struct timeval *tv)
{
    static int (*real)(int, fd_set *, fd_set *, fd_set *, struct timeval *);
    int res;

    if (real == NULL)
	real = (int (*)(int, fd_set *, fd_set *, fd_set *, struct timeval *))dlsym(RTLD_NEXT, REAL_SELECT);
    res = real(n, r, w, e, tv);
    if (res > 0 && !filled && fullmode()) {
	int cap = 0, fd;
	const char *lf = getenv("FDLOST_LOG");
	char line[64];

	filled = 1;
	filledAt = time(NULL);
	/* the log file stays open (it counts as one of the descriptors that fill the table)
	   so that it can be written once the table is full */
	if (lf != NULL)
	    logfd = open(lf, O_WRONLY | O_APPEND | O_CREAT, 0644);
	for (;;) {
	    if ((fd = open("/dev/null", O_RDONLY)) < 0)
		break;
	    if (nfill == cap) {
		int *nf = realloc(fill, (cap = cap ? cap * 2 : 256) * sizeof(int));

		if (nf == NULL) {
		    close(fd);
		    break;
		}
		fill = nf;
	    }
	    fill[nfill++] = fd;
	}
	snprintf(line, sizeof(line), "fdlost: table full (%d descriptors)\n", nfill);
	fputs(line, stderr);
	note(logfd, line);
    } else if (nfill > 0 && time(NULL) - filledAt >= 3) {
	unfill();
    }
    return(res);
}

__attribute__((constructor)) static void
init(void)
{
    mainPid = getpid();
}

/* returns 1 if the message carries a descriptor */
static int
hasfd(struct msghdr *msg)
{
    struct cmsghdr *cm;

    return(msg->msg_controllen >= CMSG_LEN(sizeof(int)) &&
	(cm = CMSG_FIRSTHDR(msg)) != NULL &&
	cm->cmsg_level == SOL_SOCKET && cm->cmsg_type == SCM_RIGHTS);
}

/* close a received descriptor; returns 1 if there was one */
static int
dropfd(struct msghdr *msg)
{
    struct cmsghdr *cm;
    int fd;

    if (msg->msg_controllen < CMSG_LEN(sizeof(int)) ||
	(cm = CMSG_FIRSTHDR(msg)) == NULL ||
	cm->cmsg_level != SOL_SOCKET || cm->cmsg_type != SCM_RIGHTS)
	return(0);
    memcpy(&fd, CMSG_DATA(cm), sizeof(fd));
    close(fd);
    msg->msg_controllen = 0;
    return(1);
}

ssize_t
recvmsg(int s, struct msghdr *msg, int flags)
{
    static ssize_t (*real)(int, struct msghdr *, int);
    const char *flag = getenv("FDLOST_FLAG");
    const char *mode = getenv("FDLOST_MODE");
    socklen_t clen = msg->msg_controllen;
    ssize_t r;

    if (real == NULL)
	real = (ssize_t (*)(int, struct msghdr *, int))dlsym(RTLD_NEXT, REAL_RECVMSG);
    if (getpid() == mainPid && nfill > 0) {
	int probe = nfill > 0 ? fcntl(fill[0], F_DUPFD, 0) : -1;

	unfill();
	if (probe >= 0)
	    close(probe);
	r = real(s, msg, flags);
	if (r > 0 && hasfd(msg)) {
	    if (probe < 0) {
		dropfd(msg);
		fprintf(stderr, "fdlost: message lost\n");
		note(-1, "fdlost: message lost\n");
		errno = EMFILE;
		return(-1);
	    }
	} else {
	    filled = 0;	/* no hand-back with a descriptor (stats): fill again */
	}
	return(r);
    }
    if (getpid() != mainPid || flag == NULL || access(flag, F_OK) != 0 ||
	(mode && strcmp(mode, "full") == 0))
	return(real(s, msg, flags));
    if (mode && strcmp(mode, "emfile") == 0) {
	if (pending) {
	    pending = 0;
	    if ((r = real(s, msg, flags)) > 0)
		dropfd(msg);
	    return(r);
	}
	/* look first: fail a message with a descriptor, leave it queued */
	if ((r = real(s, msg, flags | MSG_PEEK)) > 0 && dropfd(msg)) {
	    pending = 1;
	    errno = EMFILE;
	    return(-1);
	}
	msg->msg_controllen = clen;
	msg->msg_flags = 0;
	return(real(s, msg, flags));
    }
    if ((r = real(s, msg, flags)) > 0 && dropfd(msg))
	msg->msg_flags |= MSG_CTRUNC;
    return(r);
}
`

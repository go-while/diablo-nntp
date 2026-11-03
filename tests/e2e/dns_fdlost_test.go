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
			s := NewReaderSite(t, ReaderOptions{Spool: spool, Access: tc.access, NoStart: true,
				Config: map[string]string{"path_generallog": "%s/general.log"}})
			s.Env = append(s.Env, lockFaultEnv(so), "FDLOST_FLAG="+flag, "FDLOST_MODE="+tc.mode)
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

			if err := os.WriteFile(flag, nil, 0o644); err != nil {
				t.Fatal(err)
			}
			c := s.Reader(t)
			if tc.access == "" {
				expect(t, c, 205, "QUIT")
			} else {
				expect(t, c, 381, "AUTHINFO USER fred")
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
				// illumos syslog has no LOG_PERROR: the line is in the log file there
				logged, _ := os.ReadFile(filepath.Join(s.Dir, "log", "general.log"))
				if !strings.Contains(string(logged)+s.Proc.Stderr(), "recvmsg error (Too many open files)") {
					t.Errorf("no \"recvmsg error (Too many open files)\" in log/general.log or stderr:\n%s", logged)
				}
			}

			if err := os.Remove(flag); err != nil {
				t.Fatal(err)
			}
			c = s.Reader(t)
			if tc.access != "" {
				expect(t, c, 381, "AUTHINFO USER fred")
				expect(t, c, 281, "AUTHINFO PASS fredspass")
			}
			expect(t, c, 211, "GROUP e2e.test")
			expect(t, c, 205, "QUIT")
			act0("after the next session")
		})
	}
}

// fdLostSource is the shim: in the process that loaded it first (the main dreaderd
// process; the reader forks inherit the recorded pid and are left alone), while the file
// FDLOST_FLAG exists, a descriptor received with recvmsg() is closed and reported as lost
// (no control message, MSG_CTRUNC; Linux). With FDLOST_MODE=emfile a message that carries a
// descriptor first fails with EMFILE and stays queued, and the next recvmsg() returns it
// without the descriptor and without MSG_CTRUNC (FreeBSD).
const fdLostSource = `#if defined(__sun)
#define _XOPEN_SOURCE 600	/* as diablo is built: recvmsg() is __xnet_recvmsg() */
#define __EXTENSIONS__
#define REAL_RECVMSG "__xnet_recvmsg"
#else
#define _GNU_SOURCE
#define REAL_RECVMSG "recvmsg"
#endif
#include <sys/types.h>
#include <sys/socket.h>
#include <dlfcn.h>
#include <errno.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>

static pid_t mainPid;
static int pending;	/* emfile: the next message is returned without its descriptor */

__attribute__((constructor)) static void
init(void)
{
    mainPid = getpid();
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
    if (getpid() != mainPid || flag == NULL || access(flag, F_OK) != 0)
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

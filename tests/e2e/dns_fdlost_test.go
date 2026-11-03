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
// to the main process for the authentication.
func TestReaderHandbackFdLostReleased(t *testing.T) {
	so := preloadShim(t, "fdlost", fdLostSource)
	for _, tc := range []struct {
		name, access string
	}{
		{"quit", ""},
		{"authinfo", AuthReaderAccess},
	} {
		t.Run(tc.name, func(t *testing.T) {
			flag := filepath.Join(t.TempDir(), "lose")
			_, spool := spoolStore(t)
			s := NewReaderSite(t, ReaderOptions{Spool: spool, Access: tc.access, NoStart: true})
			s.Env = append(s.Env, lockFaultEnv(so), "FDLOST_FLAG="+flag)
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
// process; the reader forks inherit the recorded pid and are left alone), a descriptor
// received with recvmsg() is closed and reported as lost (no control message, MSG_CTRUNC)
// while the file FDLOST_FLAG exists.
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
#include <stdlib.h>
#include <string.h>
#include <unistd.h>

static pid_t mainPid;

__attribute__((constructor)) static void
init(void)
{
    mainPid = getpid();
}

ssize_t
recvmsg(int s, struct msghdr *msg, int flags)
{
    static ssize_t (*real)(int, struct msghdr *, int);
    const char *flag = getenv("FDLOST_FLAG");
    struct cmsghdr *cm;
    ssize_t r;

    if (real == NULL)
	real = (ssize_t (*)(int, struct msghdr *, int))dlsym(RTLD_NEXT, REAL_RECVMSG);
    r = real(s, msg, flags);
    if (r <= 0 || getpid() != mainPid || flag == NULL || access(flag, F_OK) != 0)
	return(r);
    if (msg->msg_controllen >= CMSG_LEN(sizeof(int)) &&
	(cm = CMSG_FIRSTHDR(msg)) != NULL &&
	cm->cmsg_level == SOL_SOCKET && cm->cmsg_type == SCM_RIGHTS) {
	int fd;

	memcpy(&fd, CMSG_DATA(cm), sizeof(fd));
	close(fd);
	msg->msg_controllen = 0;
	msg->msg_flags |= MSG_CTRUNC;
    }
    return(r);
}
`

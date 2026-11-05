/*
 * tmrshim.c - LD_PRELOAD shim for TestReaderTooManyRefsHostLimit
 * (dnsoverload_test.go).  Linux, FreeBSD, NetBSD.
 *
 * In the process that accept()s (the dreaderd main process), the first
 * TMR_N sendmsg() calls that pass a descriptor fail with ETOOMANYREFS, as
 * when too many descriptors are in flight.
 */
#define _GNU_SOURCE
#include <dlfcn.h>
#include <errno.h>
#include <stdlib.h>
#include <sys/socket.h>
#include <unistd.h>

static pid_t mainpid;
static int failed;

int accept(int fd, struct sockaddr *a, socklen_t *l) {
    static int (*real)(int, struct sockaddr *, socklen_t *);
    if (!real) real = dlsym(RTLD_NEXT, "accept");
    mainpid = getpid();
    return real(fd, a, l);
}

ssize_t sendmsg(int fd, const struct msghdr *m, int flags) {
    static ssize_t (*real)(int, const struct msghdr *, int);
    const char *e = getenv("TMR_N");
    int n = e ? atoi(e) : 0;
    if (!real) real = dlsym(RTLD_NEXT, "sendmsg");
    if (mainpid == getpid() && m->msg_controllen > 0 && failed < n) {
        ++failed;
        errno = ETOOMANYREFS;
        return -1;
    }
    return real(fd, m, flags);
}

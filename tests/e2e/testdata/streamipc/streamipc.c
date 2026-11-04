/*
 * streamipc.c - LD_PRELOAD shim for TestReaderDNSQueueStreamIPC
 * (dnsqueue_test.go).  Linux, FreeBSD, NetBSD.
 *
 * Makes dreaderd's IPC socketpairs look like illumos ones: socketpair()
 * of AF_UNIX SOCK_SEQPACKET fails (illumos does not implement it), so
 * dreaderd falls back to SOCK_STREAM; and SO_SNDBUF/SO_RCVBUF on AF_UNIX
 * sockets are set to STREAMIPC_BUF bytes instead of what dreaderd asks for
 * (illumos keeps about 16 KiB per direction whatever is set).  Each refused
 * socketpair() appends a line to STREAMIPC_LOG.
 */
#define _GNU_SOURCE
#include <dlfcn.h>
#include <errno.h>
#include <fcntl.h>
#include <stdio.h>
#include <stdlib.h>
#include <sys/socket.h>
#include <unistd.h>

int socketpair(int domain, int type, int protocol, int sv[2]) {
    static int (*real)(int, int, int, int *);
    if (!real) real = dlsym(RTLD_NEXT, "socketpair");
    if (domain == AF_UNIX && (type & 0xff) == SOCK_SEQPACKET) {
        const char *l = getenv("STREAMIPC_LOG");
        if (l != NULL) {
            int fd = open(l, O_WRONLY | O_APPEND | O_CREAT, 0644);
            if (fd >= 0) {
                char b[64];
                int n = snprintf(b, sizeof(b), "seqpacket refused pid %d\n", (int)getpid());
                if (write(fd, b, n) < 0) { /* nothing to do */ }
                close(fd);
            }
        }
        errno = EPROTONOSUPPORT;
        return -1;
    }
    return real(domain, type, protocol, sv);
}

int setsockopt(int fd, int level, int name, const void *val, socklen_t len) {
    static int (*real)(int, int, int, const void *, socklen_t);
    const char *e = getenv("STREAMIPC_BUF");
    if (!real) real = dlsym(RTLD_NEXT, "setsockopt");
    if (e != NULL && level == SOL_SOCKET && (name == SO_SNDBUF || name == SO_RCVBUF)) {
        struct sockaddr_storage ss;
        socklen_t sl = sizeof(ss);
        if (getsockname(fd, (struct sockaddr *)&ss, &sl) == 0 && ss.ss_family == AF_UNIX) {
            int sz = atoi(e);
            return real(fd, level, name, &sz, sizeof(sz));
        }
    }
    return real(fd, level, name, val, len);
}

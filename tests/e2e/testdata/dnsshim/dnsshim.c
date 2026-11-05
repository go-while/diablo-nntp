/*
 * dnsshim.c - LD_PRELOAD resolver shim for the dreaderd DNS cache tests
 * (dnscache_test.go).  Linux (glibc or musl), FreeBSD, NetBSD.
 *
 * Fakes reverse and forward lookups so the tests do not depend on the host
 * resolver, and logs every faked lookup:
 *   127.a.b.c (not 127.0.0.1, a != 9):
 *     getnameinfo(NI_NAMEREQD) -> "h-a-b-c.bench.test"
 *     getaddrinfo("h-a-b-c.bench.test") -> 127.a.b.c
 *   127.9.b.c: getnameinfo fails with EAI_NONAME (no PTR record)
 *   127.8.b.c: getnameinfo fails with EAI_AGAIN (resolver timeout)
 *   127.5.b.c: getnameinfo fails with EAI_AGAIN once per address, then works
 *   127.7.b.c: name "x-7-b-c.bench.test", whose forward lookup gives
 *              127.66.66.66 (forward mismatch)
 *   127.6.b.c: name "t-6-b-c.bench.test", whose forward lookup fails with
 *              EAI_AGAIN
 *   ::1: getnameinfo -> "v6-1.bench.test", getaddrinfo("v6-1.bench.test") -> ::1
 * Everything else goes to libc.  Forward results are built by the real
 * getaddrinfo from the numeric address, so libc's freeaddrinfo can free them
 * (musl's cannot free a hand-built block).
 *
 * Env:
 *   DSHIM_DELAY_MS  delay per faked lookup (default 0)
 *   DSHIM_COUNT     path: append "rev <addr>" or "fwd <name>" per faked lookup
 */
#define _GNU_SOURCE
#include <dlfcn.h>
#include <fcntl.h>
#include <netdb.h>
#include <netinet/in.h>
#include <arpa/inet.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <time.h>
#include <unistd.h>
#include <sys/socket.h>

static void slow(void) {
    const char *e = getenv("DSHIM_DELAY_MS");
    int ms = e ? atoi(e) : 0;
    if (ms > 0) {
        struct timespec ts = { ms / 1000, (ms % 1000) * 1000000L };
        while (nanosleep(&ts, &ts) != 0) ;
    }
}

static void count(const char *what, const char *arg) {
    const char *p = getenv("DSHIM_COUNT");
    char line[512];
    int fd, n;
    if (!p) return;
    n = snprintf(line, sizeof(line), "%s %s\n", what, arg);
    if ((fd = open(p, O_WRONLY | O_APPEND | O_CREAT, 0644)) >= 0) {
        if (write(fd, line, n) < 0) {}
        close(fd);
    }
}

/* the type of the length arguments of getnameinfo(): size_t on FreeBSD */
#ifdef __FreeBSD__
typedef size_t ni_len_t;
#else
typedef socklen_t ni_len_t;
#endif

int getnameinfo(const struct sockaddr *sa, socklen_t salen, char *host, ni_len_t hostlen,
                char *serv, ni_len_t servlen, int flags) {
    static int (*real)(const struct sockaddr *, socklen_t, char *, ni_len_t, char *, ni_len_t, int);
    if (!real) real = dlsym(RTLD_NEXT, "getnameinfo");
    if (sa && host && hostlen > 0 && !(flags & NI_NUMERICHOST) && serv == NULL) {
        if (sa->sa_family == AF_INET) {
            unsigned a = ntohl(((const struct sockaddr_in *)sa)->sin_addr.s_addr);
            char ip[32];
            if ((a >> 24) == 127 && a != 0x7f000001) {
                snprintf(ip, sizeof(ip), "127.%u.%u.%u", (a >> 16) & 255, (a >> 8) & 255, a & 255);
                count("rev", ip);
                slow();
                unsigned net = (a >> 16) & 255;
                char pfx = 'h';
                if (net == 9)
                    return EAI_NONAME;
                if (net == 8)
                    return EAI_AGAIN;
                if (net == 5) {
                    /* the first lookup of each address fails: a marker file remembers it */
                    const char *p = getenv("DSHIM_COUNT");
                    char m[600];
                    int fd;
                    snprintf(m, sizeof(m), "%s.once-%s", p ? p : "/tmp/dnsshim", ip);
                    if ((fd = open(m, O_WRONLY | O_CREAT | O_EXCL, 0644)) >= 0) {
                        close(fd);
                        return EAI_AGAIN;
                    }
                }
                if (net == 7)
                    pfx = 'x';
                if (net == 6)
                    pfx = 't';
                snprintf(host, hostlen, "%c-%u-%u-%u.bench.test", pfx, net, (a >> 8) & 255, a & 255);
                return 0;
            }
        } else if (sa->sa_family == AF_INET6 &&
                   IN6_IS_ADDR_LOOPBACK(&((const struct sockaddr_in6 *)sa)->sin6_addr)) {
            count("rev", "::1");
            slow();
            snprintf(host, hostlen, "v6-1.bench.test");
            return 0;
        }
    }
    return real(sa, salen, host, hostlen, serv, servlen, flags);
}

int getaddrinfo(const char *node, const char *service, const struct addrinfo *hints, struct addrinfo **res) {
    static int (*real)(const char *, const char *, const struct addrinfo *, struct addrinfo **);
    if (!real) real = dlsym(RTLD_NEXT, "getaddrinfo");
    unsigned a, b, c;
    char tail[64], num[64];
    struct addrinfo nh;
    memset(&nh, 0, sizeof(nh));
    nh.ai_flags = AI_NUMERICHOST;
    nh.ai_socktype = SOCK_STREAM;
    if (node && strcmp(node, "v6-1.bench.test") == 0) {
        count("fwd", node);
        slow();
        nh.ai_family = AF_INET6;
        return real("::1", service, &nh, res);
    }
    char pfx;
    if (node && sscanf(node, "%c-%u-%u-%u.%63s", &pfx, &a, &b, &c, tail) == 5 &&
        (pfx == 'h' || pfx == 'x' || pfx == 't') && strcmp(tail, "bench.test") == 0) {
        count("fwd", node);
        slow();
        if (pfx == 't')
            return EAI_AGAIN;
        if (pfx == 'x')
            a = b = c = 66;
        snprintf(num, sizeof(num), "127.%u.%u.%u", a, b, c);
        nh.ai_family = AF_INET;
        return real(num, service, &nh, res);
    }
    return real(node, service, hints, res);
}

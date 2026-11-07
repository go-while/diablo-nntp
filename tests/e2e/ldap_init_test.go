package e2e

// OpenLDAP canonicalizes the machine's own hostname on first use, even for
// a numeric LDAP URL. That process-global initialization belongs before the
// per-login deadline, not inside the first LDAP authentication attempt.

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	testkit "github.com/go-while/nntp-testkit"
)

func ldapInitEnv(t *testing.T) ([]string, string) {
	t.Helper()
	builtWith(t, "LDAP bind failed", "LDAP support (LDAP_ENABLED)")
	host, err := os.Hostname()
	if err != nil {
		t.Fatal(err)
	}
	so := preloadShim(t, "ldapinit", ldapInitSource)
	marks := filepath.Join(t.TempDir(), "init.log")
	return []string{lockFaultEnv(so), "LDAPINIT_HOST=" + host,
		"LDAPINIT_LOG=" + marks, "LDAPINIT_DELAY=" + strconv.Itoa(authTimeout+3)}, marks
}

func ldapInitLog(t *testing.T, marks string) string {
	t.Helper()
	b, err := os.ReadFile(marks)
	if os.IsNotExist(err) {
		return ""
	}
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func wantLDAPInitOnce(t *testing.T, marks string) {
	t.Helper()
	l := ldapInitLog(t, marks)
	if strings.Count(l, "begin\n") != 1 || strings.Count(l, "end\n") != 1 {
		t.Fatalf("want one completed LDAP global initialization, got:\n%s", l)
	}
}

func TestLDAPInitSlowHostname(t *testing.T) {
	env, marks := ldapInitEnv(t)
	port := slowLDAPServer(t, 0, 0, 0)
	s := authTimeoutSite(t, slowAuthAccess(ldapURL(port)), env...)
	// The injected five-second lookup exceeds the unchanged two-second
	// login limit and its two-second alarm grace. Before the fix the first
	// login gets 481, although every LDAP protocol step answers immediately.
	backendLogin(t, s, "x@slow")
	backendLogin(t, s, "x@slow")
	wantLDAPInitOnce(t, marks)
	wantTimeoutLog(t, s, "LDAP", 0, false)
}

func TestLDAPInitWithoutLDAP(t *testing.T) {
	env, marks := ldapInitEnv(t)
	s := authTimeoutSite(t, slowAuthAccess("user other\n    pass other"), env...)
	goodLogin(t, s)
	if l := ldapInitLog(t, marks); l != "" {
		t.Fatalf("LDAP initialized without a configured LDAP authdef:\n%s", l)
	}
}

func TestLDAPInitOnAccessReload(t *testing.T) {
	env, marks := ldapInitEnv(t)
	s := authTimeoutSite(t, slowAuthAccess("user other\n    pass other"), env...)
	goodLogin(t, s)
	if l := ldapInitLog(t, marks); l != "" {
		t.Fatalf("LDAP initialized before it was configured:\n%s", l)
	}
	// The compiled cache's lock timestamp has one-second resolution.
	time.Sleep(1100 * time.Millisecond)
	port := slowLDAPServer(t, 0, 0, 0)
	s.WriteFile(t, "dreader.access", slowAuthAccess(ldapURL(port)))
	testkit.WaitFor(t, testkit.Deadline(), "LDAP initialization after access reload", func() bool {
		goodLogin(t, s) // each connection makes main check for a new access file
		return strings.Contains(ldapInitLog(t, marks), "end\n")
	})
	backendLogin(t, s, "x@slow")
	wantLDAPInitOnce(t, marks)
	wantTimeoutLog(t, s, "LDAP", 0, false)
}

// Delay only the canonical lookup of this machine's hostname. LDAP's actual
// numeric endpoint and TCP/BER exchange are untouched. Resolve the local
// identity numerically after the delay, so the regression needs no live DNS.
const ldapInitSource = `
#define _GNU_SOURCE
#include <dlfcn.h>
#include <fcntl.h>
#include <netdb.h>
#include <stdlib.h>
#include <string.h>
#include <time.h>
#include <unistd.h>

static void mark(const char *text) {
    const char *path = getenv("LDAPINIT_LOG");
    int fd;
    if (path && (fd = open(path, O_WRONLY | O_APPEND | O_CREAT, 0600)) >= 0) {
        (void)write(fd, text, strlen(text));
        close(fd);
    }
}

int getaddrinfo(const char *node, const char *service,
    const struct addrinfo *hints, struct addrinfo **res) {
    static int (*real)(const char *, const char *, const struct addrinfo *, struct addrinfo **);
    const char *host = getenv("LDAPINIT_HOST");
    const char *delay = getenv("LDAPINIT_DELAY");
    int r;
    if (!real) real = dlsym(RTLD_NEXT, "getaddrinfo");
    if (host && node && !strcmp(host, node) && hints && (hints->ai_flags & AI_CANONNAME)) {
        struct timespec ts = { delay ? atoi(delay) : 0, 0 };
        mark("begin\n");
        while (nanosleep(&ts, &ts) != 0) {}
        r = real("127.0.0.1", service, hints, res);
        mark("end\n");
        return r;
    }
    return real(node, service, hints, res);
}
`

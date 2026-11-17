package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	testkit "github.com/go-while/nntp-testkit"
)

// hostCacheGAISource is preloaded into diablo: while the file named by HC_DNSFAIL exists,
// getaddrinfo() of a host name (not a numeric address) fails, with EAI_NONAME when the file
// starts with "N" (the name does not exist) and with EAI_AGAIN otherwise (the resolver does
// not answer).
const hostCacheGAISource = `#define _GNU_SOURCE
#include <ctype.h>
#include <dlfcn.h>
#include <fcntl.h>
#include <netdb.h>
#include <stdarg.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>
#include <sys/socket.h>
/*MARK*/
int
getaddrinfo(const char *node, const char *service, const struct addrinfo *hints, struct addrinfo **res)
{
    static int (*real)(const char *, const char *, const struct addrinfo *, struct addrinfo **);
    const char *flag = getenv("HC_DNSFAIL");
    char c = 0;
    int fd;

    if (real == NULL)
	real = (int (*)(const char *, const char *, const struct addrinfo *, struct addrinfo **))
	    dlsym(RTLD_NEXT, "getaddrinfo");
    if (node != NULL && isalpha((unsigned char)node[0]) && flag != NULL &&
					(fd = open(flag, O_RDONLY)) >= 0) {
	(void)read(fd, &c, 1);
	close(fd);
	if (c == 'N') {
	    shim_mark("EAI_NONAME %s", node);
	    return EAI_NONAME;
	}
	shim_mark("EAI_AGAIN %s", node);
	return EAI_AGAIN;
    }
    return real(node, service, hints, res);
}
`

// TestHostCacheDNSFailureKeepsPeers (deep review r2 M9, D-236): the host cache is rebuilt
// when diablo.hosts or dnewsfeeds changes and at least once an hour, and the new dhosts.cache
// replaces the old one. An inhost given by name got its records only when getaddrinfo()
// answered, so a resolver that did not answer during a rebuild locked every peer named by
// host name out until the next rebuild (up to an hour), although DNS was back a moment
// later. On such a temporary failure the rebuild now keeps the addresses the name had in the
// previous cache. The peer "localhost" (127.0.0.1) must be let in (200) during the failure
// and after it. A name that does not exist (EAI_NONAME) is still dropped, and so is a peer
// removed from dnewsfeeds during a failure: nothing is kept that the configuration and DNS
// no longer give.
func TestHostCacheDNSFailureKeepsPeers(t *testing.T) {
	so := preloadShim(t, "hcgai", withMark(hostCacheGAISource))
	mark := filepath.Join(t.TempDir(), "mark")
	flag := filepath.Join(t.TempDir(), "dnsfail")
	named := DefaultNewsfeeds + "label named\n    inhost localhost\n    addgroup *\nend\n"
	s := NewSite(t, SiteOptions{
		Newsfeeds: named,
		Files:     map[string]string{"diablo.hosts": ""},
		Env:       []string{lockFaultEnv(so), "SHIM_MARK=" + mark, "HC_DNSFAIL=" + flag},
	})
	t.Cleanup(func() {
		if t.Failed() {
			t.Logf("marks:\n%s", strings.Join(shimMarks(mark), "\n"))
		}
	})
	cache := filepath.Join(s.Dir, "dhosts.cache")
	hosts := filepath.Join(s.Dir, "diablo.hosts")

	greeting := func(what string) string {
		t.Helper()
		c := s.dialFrom(t, "127.0.0.1")
		defer c.Close()
		t.Logf("%s: %q", what, c.Greeting)
		return c.Greeting
	}
	mtime := func() time.Time {
		t.Helper()
		st, err := os.Stat(cache)
		if err != nil {
			t.Fatal(err)
		}
		return st.ModTime()
	}
	// rebuild makes diablo rebuild the cache (a changed diablo.hosts or dnewsfeeds, checked
	// every 10 s; change is called with the step's number for distinct mtimes) and waits for
	// the new cache and, when markWant is set, for that mark of the rebuild's lookup.
	rebuild := func(step int, change func(int), markWant string) {
		t.Helper()
		before := mtime()
		n := len(shimMarked(mark, markWant))
		change(step)
		testkit.WaitFor(t, 40*time.Second, "host cache rebuild "+markWant, func() bool {
			st, err := os.Stat(cache)
			return err == nil && st.ModTime().After(before) &&
				(markWant == "" || len(shimMarked(mark, markWant)) > n)
		})
		time.Sleep(time.Second) // the rename is done once the mtime changed; let the child exit
	}
	touchHosts := func(step int) {
		future := time.Now().Add(time.Duration(step) * time.Minute)
		if err := os.Chtimes(hosts, future, future); err != nil {
			t.Fatal(err)
		}
	}
	dns := func(mode string) {
		t.Helper()
		if mode == "" {
			if err := os.Remove(flag); err != nil && !os.IsNotExist(err) {
				t.Fatal(err)
			}
			return
		}
		if err := os.WriteFile(flag, []byte(mode), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	if g := greeting("setup"); !strings.HasPrefix(g, "200 ") {
		t.Fatalf("setup: the peer localhost (127.0.0.1) got %q, want 200", g)
	}

	// 1. the resolver does not answer during the rebuild: the addresses are kept
	dns("AGAIN")
	rebuild(1, touchHosts, "EAI_AGAIN localhost")
	if g := greeting("rebuilt during a DNS failure, DNS still failing"); !strings.HasPrefix(g, "200 ") {
		t.Errorf("the peer localhost after a rebuild during a DNS failure: %q, want 200", g)
	}
	if !strings.Contains(generalLog(s), "localhost: ") || !strings.Contains(generalLog(s), "previous address(es) kept") {
		t.Errorf("no log line about the kept addresses of localhost in the general log")
	}
	dns("")
	if g := greeting("DNS working again"); !strings.HasPrefix(g, "200 ") {
		t.Errorf("the peer localhost after a DNS failure during the rebuild, DNS working again: %q, want 200 (locked out until the next rebuild, up to an hour)", g)
	}

	// 2. the name does not exist: dropped, as before
	dns("N")
	rebuild(2, touchHosts, "EAI_NONAME localhost")
	dns("")
	if g := greeting("rebuilt while the name does not exist"); !strings.HasPrefix(g, "502 ") {
		t.Errorf("the peer localhost after a rebuild that found no such name: %q, want 502", g)
	}

	// back to normal, then the peer is removed from dnewsfeeds during a DNS failure
	rebuild(3, touchHosts, "")
	if g := greeting("rebuilt with DNS working"); !strings.HasPrefix(g, "200 ") {
		t.Fatalf("the peer localhost after a rebuild with DNS working: %q, want 200", g)
	}
	dns("AGAIN")
	rebuild(4, func(int) { s.WriteFile(t, "dnewsfeeds", DefaultNewsfeeds) }, "")
	dns("")
	if g := greeting("removed from dnewsfeeds during a DNS failure"); !strings.HasPrefix(g, "502 ") {
		t.Errorf("the peer localhost removed from dnewsfeeds during a DNS failure: %q, want 502", g)
	}
}

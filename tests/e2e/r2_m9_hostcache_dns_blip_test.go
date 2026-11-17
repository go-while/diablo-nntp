package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	testkit "github.com/go-while/nntp-testkit"
)

// gaiFailSource is preloaded into diablo: while the file named by R2_DNSFAIL exists,
// getaddrinfo() of a host name (not a numeric address) fails with EAI_AGAIN, as when the
// resolver does not answer.
const gaiFailSource = `#define _GNU_SOURCE
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
    const char *flag = getenv("R2_DNSFAIL");

    if (real == NULL)
	real = (int (*)(const char *, const char *, const struct addrinfo *, struct addrinfo **))
	    dlsym(RTLD_NEXT, "getaddrinfo");
    if (node != NULL && isalpha((unsigned char)node[0]) && flag != NULL && access(flag, F_OK) == 0) {
	shim_mark("EAI_AGAIN %s", node);
	return EAI_AGAIN;
    }
    return real(node, service, hints, res);
}
`

// TestR2HostCacheDNSBlip (deep review r2 M9): convHost() (lib/hostauth.c) writes the records
// of an inhost given by name only when getaddrinfo() succeeds, and the new dhosts.cache
// replaces the old one; the next rebuild is up to an hour later. So a resolver that does not
// answer during a rebuild locks out every peer named by host name until then, although DNS
// is back a moment later. Here the peer "localhost" (127.0.0.1) connects after a rebuild
// during a DNS failure, with DNS working again: it must still be let in (200).
func TestR2HostCacheDNSBlip(t *testing.T) {
	so := preloadShim(t, "gaifail", withMark(gaiFailSource))
	mark := filepath.Join(t.TempDir(), "mark")
	flag := filepath.Join(t.TempDir(), "dnsfail")
	s := NewSite(t, SiteOptions{
		Newsfeeds: DefaultNewsfeeds + "label named\n    inhost localhost\n    addgroup *\nend\n",
		Files:     map[string]string{"diablo.hosts": ""},
		Env:       []string{lockFaultEnv(so), "SHIM_MARK=" + mark, "R2_DNSFAIL=" + flag},
	})
	t.Cleanup(func() {
		if t.Failed() {
			t.Logf("marks:\n%s", strings.Join(shimMarks(mark), "\n"))
		}
	})
	c := s.dialFrom(t, "127.0.0.1")
	if !strings.HasPrefix(c.Greeting, "200 ") {
		t.Fatalf("setup: the peer localhost (127.0.0.1) got %q, want 200", c.Greeting)
	}
	c.ExpectCmd(205, "QUIT")

	cache := filepath.Join(s.Dir, "dhosts.cache")
	st, err := os.Stat(cache)
	if err != nil {
		t.Fatal(err)
	}
	before := st.ModTime()
	if err := os.WriteFile(flag, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	// a changed diablo.hosts makes diablo rebuild the cache (checked every 10 s)
	future := time.Now().Add(time.Minute)
	if err := os.Chtimes(filepath.Join(s.Dir, "diablo.hosts"), future, future); err != nil {
		t.Fatal(err)
	}
	testkit.WaitFor(t, 40*time.Second, "a host cache rebuild during the DNS failure", func() bool {
		st, err := os.Stat(cache)
		return err == nil && st.ModTime().After(before) && len(shimMarked(mark, "EAI_AGAIN localhost")) > 0
	})
	time.Sleep(time.Second) // the rename is done once the mtime changed; let the child exit
	if err := os.Remove(flag); err != nil {
		t.Fatal(err)
	}

	c = s.dialFrom(t, "127.0.0.1")
	defer c.Close()
	if !strings.HasPrefix(c.Greeting, "200 ") {
		t.Errorf("the peer localhost after a DNS failure during the rebuild, DNS working again: %q, want 200 (locked out until the next rebuild, up to an hour)", c.Greeting)
	}
}

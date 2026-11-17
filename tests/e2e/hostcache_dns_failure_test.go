package e2e

import (
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	testkit "github.com/go-while/nntp-testkit"
)

// hostCacheGAISource is preloaded into diablo: while the file named by HC_DNSFAIL exists,
// getaddrinfo() of a host name (not a numeric address) does what the file's first byte says:
// "N" fails with EAI_NONAME (no such name), "F" with EAI_FAIL (a failed answer), "S" with
// EAI_SYSTEM, "C" answers 127.0.0.3 (the name has a new address), "X" answers 127.0.0.3 to
// the first such lookup of a process and fails the later ones with EAI_AGAIN (DNS goes away
// during a rebuild), anything else fails with EAI_AGAIN (the resolver does not answer). Its first call in each process writes the mark
// "CALLED", so a test can tell that the shim is in use at all.
const hostCacheGAISource = `#define _GNU_SOURCE
#include <ctype.h>
#include <dlfcn.h>
#include <errno.h>
#include <fcntl.h>
#include <netdb.h>
#include <stdarg.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>
#include <sys/socket.h>
#include <sys/types.h>
/*MARK*/
int
getaddrinfo(const char *node, const char *service, const struct addrinfo *hints, struct addrinfo **res)
{
    static int (*real)(const char *, const char *, const struct addrinfo *, struct addrinfo **);
    static int called;
    static pid_t npid;
    static int n;
    const char *flag = getenv("HC_DNSFAIL");
    char c = 0;
    int fd;

    if (!called) {
	called = 1;
	shim_mark("CALLED");
    }

    if (real == NULL)
	real = (int (*)(const char *, const char *, const struct addrinfo *, struct addrinfo **))
	    dlsym(RTLD_NEXT, "getaddrinfo");
    if (node != NULL && isalpha((unsigned char)node[0]) && flag != NULL &&
					(fd = open(flag, O_RDONLY)) >= 0) {
	(void)read(fd, &c, 1);
	close(fd);
	if (npid != getpid()) {
	    npid = getpid();
	    n = 0;
	}
	if (c == 'X' && ++n == 1) {
	    shim_mark("CHANGED %s", node);
	    return real("127.0.0.3", service, hints, res);
	}
	switch (c) {
	case 'N':
	    shim_mark("EAI_NONAME %s", node);
	    return EAI_NONAME;
	case 'F':
	    shim_mark("EAI_FAIL %s", node);
	    return EAI_FAIL;
	case 'S':
	    shim_mark("EAI_SYSTEM %s", node);
	    errno = EIO;
	    return EAI_SYSTEM;
	case 'C':
	    shim_mark("CHANGED %s", node);
	    return real("127.0.0.3", service, hints, res);
	}
	shim_mark("EAI_AGAIN %s", node);
	return EAI_AGAIN;
    }
    return real(node, service, hints, res);
}
`

// hcSite is a feeder site with the getaddrinfo shim above and the helpers of the host
// cache tests.
type hcSite struct {
	*Site
	t     *testing.T
	mark  string
	flag  string
	cache string
	step  int
}

func newHCSite(t *testing.T, feeds, hosts string) *hcSite {
	t.Helper()
	if !preloadHost(false) {
		t.Skip("the fault injector uses LD_PRELOAD")
	}
	so := preloadShim(t, "hcgai", withMark(hostCacheGAISource))
	h := &hcSite{t: t, mark: filepath.Join(t.TempDir(), "mark"), flag: filepath.Join(t.TempDir(), "dnsfail")}
	h.Site = NewSite(t, SiteOptions{
		Newsfeeds: feeds,
		Files:     map[string]string{"diablo.hosts": hosts},
		Env:       []string{lockFaultEnv(so), "SHIM_MARK=" + h.mark, "HC_DNSFAIL=" + h.flag},
	})
	h.cache = filepath.Join(h.Dir, "dhosts.cache")
	// the start has built the host cache, which looks up localhost: the shim must have
	// seen that, or it does not take effect here and every later wait would time out
	if len(shimMarked(h.mark, "CALLED")) == 0 {
		t.Fatalf("the getaddrinfo shim was never hit (no CALLED mark after the host cache was built): it does not take effect on %s", runtime.GOOS)
	}
	t.Cleanup(func() {
		if t.Failed() {
			t.Logf("marks:\n%s", strings.Join(shimMarks(h.mark), "\n"))
		}
	})
	return h
}

// dns sets the shim's mode ("" = DNS works).
func (h *hcSite) dns(mode string) {
	h.t.Helper()
	if mode == "" {
		if err := os.Remove(h.flag); err != nil && !os.IsNotExist(err) {
			h.t.Fatal(err)
		}
		return
	}
	if err := os.WriteFile(h.flag, []byte(mode), 0o600); err != nil {
		h.t.Fatal(err)
	}
}

// greeting connects from ip and returns the greeting.
func (h *hcSite) greeting(ip, what string) string {
	h.t.Helper()
	c := h.dialFrom(h.t, ip)
	defer c.Close()
	h.t.Logf("%s, from %s: %q", what, ip, c.Greeting)
	return c.Greeting
}

// rebuild makes diablo rebuild the host cache, by a newer diablo.hosts or by change when it
// is not nil (diablo checks every 10 s), and waits for the new cache and, when markWant is
// set, for one more mark of it.
func (h *hcSite) rebuild(change func(), markWant string) {
	h.t.Helper()
	st, err := os.Stat(h.cache)
	if err != nil {
		h.t.Fatal(err)
	}
	before := st.ModTime()
	n := len(shimMarked(h.mark, markWant))
	h.step++
	if change != nil {
		change()
	} else {
		future := time.Now().Add(time.Duration(h.step) * time.Minute)
		if err := os.Chtimes(filepath.Join(h.Dir, "diablo.hosts"), future, future); err != nil {
			h.t.Fatal(err)
		}
	}
	testkit.WaitFor(h.t, 40*time.Second, "host cache rebuild "+markWant, func() bool {
		st, err := os.Stat(h.cache)
		return err == nil && st.ModTime().After(before) &&
			(markWant == "" || len(shimMarked(h.mark, markWant)) > n)
	})
	time.Sleep(time.Second) // the rename is done once the mtime changed; let the child exit
}

// records returns the addresses dhosts.cache has for entry, by label, one item per record
// (dprimehostcache -dump).
func (h *hcSite) records(entry string) map[string][]string {
	h.t.Helper()
	m := map[string][]string{}
	for _, l := range strings.Split(h.Run(h.t, "dprimehostcache", "-dump"), "\n") {
		f := strings.Fields(l)
		if len(f) < 4 || f[0] != entry {
			continue
		}
		label := strings.TrimPrefix(f[1], "label=")
		m[label] = append(m[label], strings.TrimPrefix(f[3], "ip="))
	}
	return m
}

// uniq returns the sorted distinct items of s.
func uniq(s []string) []string {
	c := slices.Clone(s)
	slices.Sort(c)
	return slices.Compact(c)
}

// TestHostCacheDNSFailureKeepsPeers (D-236): the host cache is rebuilt when diablo.hosts or
// dnewsfeeds changes and after hostcacherebuildtime (an hour by default), and the new
// dhosts.cache replaces the old one. An inhost given by name got its records only when
// getaddrinfo() answered, so a resolver that did not answer during a rebuild locked a peer
// named by host name out until the next rebuild (by default up to an hour), also after DNS
// answered again. On EAI_AGAIN, EAI_MEMORY or EAI_SYSTEM the rebuild now keeps the addresses
// the name had in the previous cache. The peer "localhost" (127.0.0.1) must be let in (200)
// during an EAI_AGAIN failure and after it, and after EAI_SYSTEM. Other errors (EAI_NONAME,
// EAI_FAIL) drop the name, and so does removing the peer from dnewsfeeds during a failure.
func TestHostCacheDNSFailureKeepsPeers(t *testing.T) {
	named := DefaultNewsfeeds + "label named\n    inhost localhost\n    addgroup *\nend\n"
	h := newHCSite(t, named, "")
	want := func(code, what string) {
		t.Helper()
		if g := h.greeting("127.0.0.1", what); !strings.HasPrefix(g, code+" ") {
			t.Errorf("the peer localhost %s: %q, want %s", what, g, code)
		}
	}

	if g := h.greeting("127.0.0.1", "setup"); !strings.HasPrefix(g, "200 ") {
		t.Fatalf("setup: the peer localhost (127.0.0.1) got %q, want 200", g)
	}

	// the resolver does not answer during the rebuild: the addresses are kept
	h.dns("AGAIN")
	h.rebuild(nil, "EAI_AGAIN localhost")
	want("200", "after a rebuild during EAI_AGAIN, DNS still failing")
	if g := generalLog(h.Site); !strings.Contains(g, "host cache: localhost: ") || !strings.Contains(g, "previous address(es) kept") {
		t.Errorf("no log line about the kept addresses of localhost in the general log")
	}
	h.dns("")
	want("200", "after a rebuild during EAI_AGAIN, DNS working again (locked out until the next rebuild before the fix)")

	// a system error: kept too
	h.dns("S")
	h.rebuild(nil, "EAI_SYSTEM localhost")
	h.dns("")
	want("200", "after a rebuild during EAI_SYSTEM")

	// no such name: dropped, as before
	h.dns("N")
	h.rebuild(nil, "EAI_NONAME localhost")
	h.dns("")
	want("502", "after a rebuild that found no such name (EAI_NONAME)")

	h.rebuild(nil, "")
	if g := h.greeting("127.0.0.1", "after a rebuild with DNS working"); !strings.HasPrefix(g, "200 ") {
		t.Fatalf("the peer localhost after a rebuild with DNS working: %q, want 200", g)
	}

	// a failed answer: dropped, as before
	h.dns("F")
	h.rebuild(nil, "EAI_FAIL localhost")
	h.dns("")
	want("502", "after a rebuild during EAI_FAIL")

	h.rebuild(nil, "")
	if g := h.greeting("127.0.0.1", "after a rebuild with DNS working"); !strings.HasPrefix(g, "200 ") {
		t.Fatalf("the peer localhost after a rebuild with DNS working: %q, want 200", g)
	}

	// the peer is removed from dnewsfeeds during a DNS failure: not kept
	h.dns("AGAIN")
	h.rebuild(func() { h.WriteFile(t, "dnewsfeeds", DefaultNewsfeeds) }, "")
	h.dns("")
	want("502", "removed from dnewsfeeds during EAI_AGAIN")
}

// TestHostCacheDNSFailureNoDuplicates (D-236): a name listed more than
// once, under two labels and twice under each (host and inhost in dnewsfeeds; inhost and a
// diablo.hosts line), keeps over two rebuilds during EAI_AGAIN exactly the addresses each
// label had, each once: the records do not multiply from one failing rebuild to the next, and
// no label gets another label's records. A later successful lookup that gives a new address
// writes only the new one, and that address is let in.
func TestHostCacheDNSFailureNoDuplicates(t *testing.T) {
	feeds := DefaultNewsfeeds +
		"label named\n    inhost localhost\n    host localhost\n    addgroup *\nend\n" +
		"label other\n    inhost localhost\n    addgroup *\nend\n"
	// other also from diablo.hosts: the name twice under it, from two files
	h := newHCSite(t, feeds, "localhost other\n")

	r0 := h.records("localhost")
	t.Logf("records before: %v", r0)
	if len(r0["named"]) == 0 || len(r0["other"]) == 0 || !slices.Contains(r0["named"], "127.0.0.1") {
		t.Fatalf("setup: dhosts.cache has %v for localhost, want 127.0.0.1 under named and other", r0)
	}

	h.dns("AGAIN")
	var r1 map[string][]string
	for i := 1; i <= 2; i++ {
		h.rebuild(nil, "EAI_AGAIN localhost")
		r := h.records("localhost")
		t.Logf("records after failing rebuild %d: %v", i, r)
		if len(r) != 2 {
			t.Errorf("failing rebuild %d: labels %v for localhost, want named and other only", i, r)
		}
		for _, label := range []string{"named", "other"} {
			if got, want := r[label], uniq(r0[label]); !slices.Equal(uniq(got), want) || len(got) != len(want) {
				t.Errorf("failing rebuild %d: label %s has %v, want each of %v once", i, label, got, want)
			}
		}
		if i == 2 && (len(r["named"]) != len(r1["named"]) || len(r["other"]) != len(r1["other"])) {
			t.Errorf("the records grew from one failing rebuild to the next: %v, then %v", r1, r)
		}
		r1 = r
	}
	h.dns("")
	if g := h.greeting("127.0.0.1", "after two failing rebuilds"); !strings.HasPrefix(g, "200 ") {
		t.Errorf("the peer localhost after two rebuilds during EAI_AGAIN: %q, want 200", g)
	}

	// the name answers again, with a new address: only that one is in the cache
	h.dns("C")
	h.rebuild(nil, "CHANGED localhost")
	h.dns("")
	r := h.records("localhost")
	t.Logf("records after the address changed: %v", r)
	for _, label := range []string{"named", "other"} {
		if got := uniq(r[label]); !slices.Equal(got, []string{"127.0.0.3"}) {
			t.Errorf("after a lookup that gave 127.0.0.3: label %s has %v, want only 127.0.0.3", label, r[label])
		}
	}
	// (127.0.0.1 itself is still let in: its reverse name localhost resolves to it again
	// with DNS working, and the entry matches by name)
	if g := h.greeting("127.0.0.3", "new address after the change"); !strings.HasPrefix(g, "200 ") {
		t.Errorf("the new address 127.0.0.3 of localhost: %q, want 200", g)
	}
}

// TestHostCacheDNSFailureAfterFreshLookup (D-236): a name
// listed twice under one label (inhost in dnewsfeeds, then a diablo.hosts line) whose first
// lookup in a rebuild answers and whose second fails with EAI_AGAIN keeps only the fresh
// addresses: the failing line must not write the old addresses next to them.
func TestHostCacheDNSFailureAfterFreshLookup(t *testing.T) {
	feeds := DefaultNewsfeeds + "label named\n    inhost localhost\n    addgroup *\nend\n"
	h := newHCSite(t, feeds, "localhost named\n")

	r0 := h.records("localhost")
	t.Logf("records before: %v", r0)
	if !slices.Contains(r0["named"], "127.0.0.1") {
		t.Fatalf("setup: dhosts.cache has %v for localhost, want 127.0.0.1 under named", r0)
	}

	// the dnewsfeeds line is looked up first and answers 127.0.0.3, the diablo.hosts line fails
	h.dns("X")
	h.rebuild(nil, "EAI_AGAIN localhost")
	h.dns("")
	if len(shimMarked(h.mark, "CHANGED localhost")) == 0 {
		t.Fatalf("the rebuild's first lookup of localhost did not answer (marks %v)", shimMarks(h.mark))
	}
	r := h.records("localhost")
	t.Logf("records after the rebuild: %v", r)
	if got := uniq(r["named"]); !slices.Equal(got, []string{"127.0.0.3"}) {
		t.Errorf("label named has %v after a rebuild whose first lookup gave 127.0.0.3 and whose second failed, want only 127.0.0.3 (no old address kept next to the fresh one)", r["named"])
	}
}

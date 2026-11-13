package e2e

import (
	"bufio"
	"net"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/go-while/nntp-testkit/client"
)

// ldapFreeSource watches what LDAPAuthenticate() gives back: it marks each search result and
// filter string the program passes to ldap_search_s()/ldap_search_ext_s(), each result it
// passes to ldap_msgfree() and the free() of the last filter.
const ldapFreeSource = `#define _GNU_SOURCE
#define __EXTENSIONS__
#include <dlfcn.h>
#include <fcntl.h>
#include <stdarg.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>
/*MARK*/
static void (*real_free)(void *);
static const void *filter;
static int depth;

__attribute__((constructor)) static void
ldapfree_init(void)
{
    real_free = (void (*)(void *))dlsym(RTLD_NEXT, "free");
}

void
free(void *p)
{
    if (p != NULL && p == filter) {
	filter = NULL;
	shim_mark("free filter %p", p);
    }
    if (real_free != NULL)
	real_free(p);
}

static void
searched(const char *f, void **res, int rc)
{
    filter = f;
    shim_mark("search rc %d filter %p result %p", rc, (void *)f, res ? *res : NULL);
}

int
ldap_search_s(void *ld, const char *base, int scope, const char *f, char **attrs,
	int attrsonly, void **res)
{
    int (*real)(void *, const char *, int, const char *, char **, int, void **) =
	(int (*)(void *, const char *, int, const char *, char **, int, void **))dlsym(RTLD_NEXT, "ldap_search_s");
    int rc;

    ++depth;
    rc = real(ld, base, scope, f, attrs, attrsonly, res);
    if (--depth == 0)
	searched(f, res, rc);
    return rc;
}

int
ldap_search_ext_s(void *ld, const char *base, int scope, const char *f, char **attrs,
	int attrsonly, void *sctrls, void *cctrls, void *timeout, int sizelimit, void **res)
{
    int (*real)(void *, const char *, int, const char *, char **, int, void *, void *, void *, int, void **) =
	(int (*)(void *, const char *, int, const char *, char **, int, void *, void *, void *, int, void **))dlsym(RTLD_NEXT, "ldap_search_ext_s");
    int rc;

    ++depth;
    rc = real(ld, base, scope, f, attrs, attrsonly, sctrls, cctrls, timeout, sizelimit, res);
    if (--depth == 0)
	searched(f, res, rc);
    return rc;
}

int
ldap_msgfree(void *lm)
{
    int (*real)(void *) = (int (*)(void *))dlsym(RTLD_NEXT, "ldap_msgfree");

    if (lm != NULL && depth == 0)
	shim_mark("msgfree %p", lm);
    return real(lm);
}
`

// deepRevLDAPSearchErrServer answers a bind with success and every search with no entry and
// the result code noSuchObject (32): ldap_search_ext_s() then fails but still hands back the
// result message.
func deepRevLDAPSearchErrServer(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { l.Close() })
	result := func(code byte) []byte {
		r := ldapTLV(0x0a, []byte{code})
		r = append(r, ldapTLV(0x04, nil)...)
		return append(r, ldapTLV(0x04, nil)...)
	}
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				r := bufio.NewReader(c)
				for {
					_, msg, err := ldapReadTLV(r)
					if err != nil || len(msg) < 3 || msg[0] != 0x02 || len(msg) < 3+int(msg[1]) {
						return
					}
					idTLV := ldapTLV(0x02, msg[2:2+int(msg[1])])
					var out []byte
					switch msg[2+int(msg[1])] {
					case 0x60: // bind
						out = ldapTLV(0x30, append(idTLV, ldapTLV(0x61, result(0))...))
					case 0x63: // search: done, noSuchObject
						out = ldapTLV(0x30, append(idTLV, ldapTLV(0x65, result(32))...))
					default:
						continue
					}
					if _, err := c.Write(out); err != nil {
						return
					}
				}
			}(c)
		}
	}()
	return l.Addr().(*net.TCPAddr).Port
}

// TestDeepRevLDAPFreesResult (L6-4): LDAPAuthenticate() (dreaderd/dns.c, the default LDAP
// code) builds a filter string with malloc() and gets the search result from the library.
// The path that reaches the compare, the one of every login with a known user, returned
// without free(filter) and ldap_msgfree(res): the DNS worker lost some hundred bytes per
// login, for as long as it ran. The return after a failed search freed the filter but not
// the result, which the library hands back also for a failed search (ldap_search_ext_s(3):
// free it whatever the return value).
//
// A preloaded library watches the search, ldap_msgfree() and free(). After a login whose
// password the directory accepts, one it refuses, and one whose search fails (noSuchObject),
// every search result must have been given back and the filter freed.
func TestDeepRevLDAPFreesResult(t *testing.T) {
	builtWith(t, "LDAP bind failed", "LDAP support (LDAP_ENABLED)")
	so := preloadShim(t, "ldapfree", withMark(ldapFreeSource))
	mark := filepath.Join(t.TempDir(), "mark")
	portA, _ := deepRevLDAPServer(t, 6) // compareTrue
	portB, _ := deepRevLDAPServer(t, 5) // compareFalse
	portC := deepRevLDAPSearchErrServer(t)
	url := func(port int) string {
		return "ldap ldap://127.0.0.1:" + strconv.Itoa(port) + "/dc=e2e,dc=test?userPassword?sub?(uid=%25s)"
	}
	access := `vserverdef DEFAULT
    hostname ` + PathHost + `
end
authdef la
    realm a
    ` + url(portA) + `
end
authdef lb
    realm b
    ` + url(portB) + `
end
authdef lc
    realm c
    ` + url(portC) + `
end
readerdef ra
    read yes
    auth la
end
readerdef rb
    read yes
    auth lb
end
readerdef rc
    read yes
    auth lc
end
access 127.0.0.1 ra
access 127.0.0.1 rb
access 127.0.0.1 rc
`
	s := authTimeoutSite(t, access, lockFaultEnv(so), "SHIM_MARK="+mark)
	t.Cleanup(func() {
		if t.Failed() {
			t.Logf("marks:\n%s", strings.Join(shimMarks(mark), "\n"))
		}
	})
	login := func(user string) int {
		t.Helper()
		c := client.DialWith(t, s.Addr, 2*authTimeoutBound)
		defer c.Close()
		c.ExpectCmd(381, "AUTHINFO USER %s", user)
		code, _ := c.Cmd("AUTHINFO PASS pw")
		return code
	}
	if code := login("x@a"); code != 281 {
		t.Fatalf("x@a (the directory accepts the password): %d, want 281", code)
	}
	if code := login("x@b"); code == 281 {
		t.Fatalf("x@b (the directory refuses the password) logged in")
	}
	if code := login("x@c"); code == 281 {
		t.Fatalf("x@c (the search fails) logged in")
	}
	time.Sleep(200 * time.Millisecond)

	var results, filters, rcs []string
	freed := map[string]bool{}
	for _, l := range shimMarks(mark) {
		f := strings.Fields(l)
		switch {
		case len(f) == 8 && f[1] == "search" && f[7] != "(nil)" && f[7] != "0x0":
			filters = append(filters, f[5])
			results = append(results, f[7])
			rcs = append(rcs, f[3])
		case len(f) == 3 && f[1] == "msgfree":
			freed["result "+f[2]] = true
		case len(f) == 4 && f[1] == "free" && f[2] == "filter":
			freed["filter "+f[3]] = true
		}
	}
	if len(results) != 3 || rcs[0] != "0" || rcs[1] != "0" || rcs[2] == "0" {
		t.Fatalf("the injector saw searches with a result and the return codes %q, want 3 (one per login), the third failed", rcs)
	}
	for i, r := range results {
		if !freed["result "+r] {
			t.Errorf("login %d: the search result %s (search returned %s) was not given back with ldap_msgfree()", i+1, r, rcs[i])
		}
		if !freed["filter "+filters[i]] {
			t.Errorf("login %d: the filter string %s was not freed", i+1, filters[i])
		}
	}
}

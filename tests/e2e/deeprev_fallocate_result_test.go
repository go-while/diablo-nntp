package e2e

import (
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	testkit "github.com/go-while/nntp-testkit"
)

// fallocFailSource stands in for a filesystem that cannot allocate space: the fallocate
// system call fails with EOPNOTSUPP, and posix_fallocate() returns EOPNOTSUPP (it returns the
// error number and leaves errno alone). Each call is marked.
const fallocFailSource = `#define _GNU_SOURCE
#include <dlfcn.h>
#include <errno.h>
#include <fcntl.h>
#include <stdarg.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/syscall.h>
#include <sys/types.h>
#include <unistd.h>
/*MARK*/
long
syscall(long n, ...)
{
    static long (*real)(long, ...);
    va_list ap;
    long a[6];
    int i;

    va_start(ap, n);
    for (i = 0; i < 6; i++)
	a[i] = va_arg(ap, long);
    va_end(ap);
    if (n == SYS_fallocate) {
	shim_mark("fallocate");
	errno = EOPNOTSUPP;
	return -1;
    }
    if (real == NULL)
	real = (long (*)(long, ...))dlsym(RTLD_NEXT, "syscall");
    return real(n, a[0], a[1], a[2], a[3], a[4], a[5]);
}

int
posix_fallocate(int fd, off_t off, off_t len)
{
    (void)fd; (void)off; (void)len;
    shim_mark("posix_fallocate");
    return EOPNOTSUPP;
}

#ifdef __GLIBC__	/* musl has no posix_fallocate64 of its own */
int
posix_fallocate64(int fd, off64_t off, off64_t len)
{
    (void)fd; (void)off; (void)len;
    shim_mark("posix_fallocate");
    return EOPNOTSUPP;
}
#endif
`

// TestDeepRevPosixFallocateResult: dreaderd allocates the space of a new overview index with
// the fallocate system call, else posix_fallocate(), else ftruncate()
// (dreaderd/filealloc.c, FileAllocSpace(), Linux only). It stops trying a call the
// filesystem does not support. posix_fallocate() returns the error number and does not set
// errno, but the test was "result < 0 and errno", which is never true: every new file asked
// posix_fallocate() again.
//
// A preloaded library makes both calls fail with EOPNOTSUPP. Articles in two groups make the
// feed process create two overview indexes: it must ask posix_fallocate() once, and the
// groups must work (the ftruncate() fallback).
func TestDeepRevPosixFallocateResult(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("dreaderd uses fallocate and posix_fallocate on Linux only")
	}
	so := preloadShim(t, "fallocfail", withMark(fallocFailSource))
	mark := filepath.Join(t.TempDir(), "mark")
	st, spool := spoolStore(t)
	st.AddGroup("e2e.other", "e2e other group")
	s := NewReaderSite(t, ReaderOptions{Spool: spool, NoStart: true})
	s.Env = append(s.Env, lockFaultEnv(so), "SHIM_MARK="+mark)
	t.Cleanup(func() {
		if t.Failed() {
			t.Logf("general.log:\n%s\nmarks:\n%s", generalLog(s), strings.Join(shimMarks(mark), "\n"))
		}
	})
	startReader(t, s, "[127.0.0.1]:"+strings.TrimPrefix(s.Addr, "127.0.0.1:"))
	feedReader(t, s, st, "e2e.test", newArticle(msgID(t), "e2e.test", "one", "body"))
	feedReader(t, s, st, "e2e.other", newArticle(msgID(t), "e2e.other", "two", "body"))

	c := s.Reader(t)
	for _, g := range []string{"e2e.test", "e2e.other"} {
		testkit.WaitFor(t, testkit.Deadline(), "one article in GROUP "+g, func() bool {
			_, line := c.Cmd("GROUP %s", g)
			return strings.HasPrefix(line, "211 1 ")
		})
	}
	c.ExpectCmd(205, "QUIT")
	time.Sleep(200 * time.Millisecond)

	calls := map[string]int{}
	for _, l := range shimMarked(mark, "posix_fallocate") {
		calls[strings.Fields(l)[0]]++
	}
	if len(calls) == 0 {
		t.Fatalf("no process called posix_fallocate(): the scenario did not develop")
	}
	for pid, n := range calls {
		if n > 1 {
			t.Errorf("process %s called posix_fallocate() %d times, after its first call had returned EOPNOTSUPP", pid, n)
		}
	}
}

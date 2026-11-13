package e2e

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	testkit "github.com/go-while/nntp-testkit"
)

// groupMapGuardSource maps cache.groupdef, when its size is a whole number of pages, with an
// inaccessible page right behind it, and marks that it did. Without it the page behind the
// mapping is whatever the kernel put there.
const groupMapGuardSource = `#define _GNU_SOURCE
#define __EXTENSIONS__
#include <dlfcn.h>
#include <errno.h>
#include <fcntl.h>
#include <stdio.h>
#include <string.h>
#include <sys/mman.h>
#include <sys/stat.h>
#include <sys/types.h>
#include <unistd.h>
/*FDPATH*/
/*MARK*/
static void *
guard_mmap(const char *name, void *addr, size_t len, int prot, int flags, int fd, off_t off)
{
    void *(*real)(void *, size_t, int, int, int, off_t) =
	(void *(*)(void *, size_t, int, int, int, off_t))dlsym(RTLD_NEXT, name);
    size_t pg = (size_t)getpagesize();
    char path[4096];
    struct stat st;

    if (fd >= 0 && addr == NULL && off == 0 && len > 0 && len % pg == 0 &&
	fstat(fd, &st) == 0 && (size_t)st.st_size == len) {
	size_t n;

	fdpath(fd, path, sizeof(path));
	n = strlen(path);
	if (n >= 15 && strcmp(path + n - 15, "/cache.groupdef") == 0) {
	    void *base = real(NULL, len + pg, PROT_NONE, MAP_PRIVATE|MAP_ANON, -1, 0);
	    void *p;

	    if (base == MAP_FAILED)
		return base;
	    p = real(base, len, prot, flags | MAP_FIXED, fd, off);
	    shim_mark("guard page behind %s (%ld bytes)", path, (long)len);
	    return p;
	}
    }
    return real(addr, len, prot, flags, fd, off);
}

void *
mmap(void *addr, size_t len, int prot, int flags, int fd, off_t off)
{
    return guard_mmap("mmap", addr, len, prot, flags, fd, off);
}

#ifdef __GLIBC__	/* with _FILE_OFFSET_BITS=64 the program calls mmap64 */
void *
mmap64(void *addr, size_t len, int prot, int flags, int fd, off64_t off)
{
    return guard_mmap("mmap64", addr, len, prot, flags, fd, (off_t)off);
}
#endif
`

// TestDeepRevGroupMapEnd (L6-6): ReadAccessCache() walks the groupdefs in cache.groupdef, a
// row of strings, until it finds an empty one; but the writer puts no empty string behind the
// last groupdef. The walk ended on the zero fill of the last page, and for a file whose size
// is a whole number of pages it read the byte behind the mapping. On Linux that byte belongs
// to whatever the kernel mapped next (here the vserver cache: one more empty groupdef, no
// visible effect).
//
// dreader.access with one groupdef of a size that makes cache.groupdef exactly one page, and a
// preloaded mmap() that puts an inaccessible page behind that mapping, as when nothing is
// mapped there: the processes that read the access cache must not fault, and a client must
// be served.
func TestDeepRevGroupMapEnd(t *testing.T) {
	so := preloadShim(t, "groupmapguard", withMark(withFdPath(groupMapGuardSource)))
	mark := filepath.Join(t.TempDir(), "mark")
	page := os.Getpagesize()

	// cache.groupdef: "DEFAULT\0" "1\0" "*\0", then the name, the count and the groups of each
	// groupdef, every one with its NUL. One groupdef "pad" with 20 groups fills the rest.
	const n = 20
	rest := page - len("DEFAULT\x001\x00*\x00") - len("pad\x00") - len(fmt.Sprint(n)) - 1
	var groups strings.Builder
	for i := 0; i < n; i++ {
		l := rest/n - 1 // without the NUL
		if i < rest%n {
			l++
		}
		name := fmt.Sprintf("e2e.pad%02d.", i)
		fmt.Fprintf(&groups, "    group %s%s\n", name, strings.Repeat("x", l-len(name)))
	}
	access := DefaultReaderAccess + "groupdef pad\n" + groups.String() + "end\n"

	_, spool := spoolStore(t)
	s := NewReaderSite(t, ReaderOptions{Spool: spool, Access: access, NoStart: true})
	s.Env = append(s.Env, lockFaultEnv(so), "SHIM_MARK="+mark)
	t.Cleanup(func() {
		if t.Failed() {
			t.Logf("general.log:\n%s\nmarks:\n%s", generalLog(s), strings.Join(shimMarks(mark), "\n"))
		}
	})
	startReaderArgs(t, s)
	var cache string
	testkit.WaitFor(t, testkit.Deadline(), "cache.groupdef", func() bool {
		filepath.WalkDir(s.Dir, func(p string, d fs.DirEntry, err error) error {
			if err == nil && d.Name() == "cache.groupdef" {
				cache = p
			}
			return nil
		})
		return cache != ""
	})
	if fi, err := os.Stat(cache); err != nil || fi.Size() != int64(page) {
		t.Fatalf("the scenario did not develop: %s is not one page (%d bytes): %v %v", cache, page, fi, err)
	}

	g := waitGreeting(s.Addr, 15*time.Second)
	if len(shimMarked(mark, "guard page behind")) == 0 {
		t.Fatalf("no process mapped cache.groupdef through the injector")
	}
	if !strings.HasPrefix(g, "200") {
		t.Fatalf("greeting %q, want 200: a process read behind the mapping of cache.groupdef", g)
	}
	c := s.Reader(t)
	c.ExpectCmd(211, "GROUP e2e.test")
	c.ExpectCmd(205, "QUIT")
	for _, bad := range []string{"Lost dns resolver", "signal 11", "Segmentation"} {
		if strings.Contains(generalLog(s), bad) {
			t.Errorf("%q in the log", bad)
		}
	}
}

package e2e

import (
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	testkit "github.com/go-while/nntp-testkit"
)

// cacheRenameSource looks at every rename() of a "*.tmp" file to a name below the directory
// CACHERENAME_DIR, right after it: it marks the size the file has under its new name.
const cacheRenameSource = `#define _GNU_SOURCE
#define __EXTENSIONS__
#include <dlfcn.h>
#include <errno.h>
#include <fcntl.h>
#include <stdarg.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/stat.h>
#include <unistd.h>
/*MARK*/
int
rename(const char *from, const char *to)
{
    static int (*real)(const char *, const char *);
    const char *dir = getenv("CACHERENAME_DIR");
    size_t n = strlen(from);
    int r;
    int e;

    if (real == NULL)
	real = (int (*)(const char *, const char *))dlsym(RTLD_NEXT, "rename");
    r = real(from, to);
    e = errno;
    if (r == 0 && dir != NULL && strncmp(to, dir, strlen(dir)) == 0 &&
	n > 4 && strcmp(from + n - 4, ".tmp") == 0) {
	struct stat st;

	if (stat(to, &st) == 0)
	    shim_mark("installed %s size %ld", to, (long)st.st_size);
    }
    errno = e;
    return r;
}
`

// TestDeepRevCacheTruncateBeforeRename (L5-02): an article read through a server with
// cache=on is written to <cache file>.tmp and renamed to the cache file when it is complete
// (dreaderd/cache.c, CommitCache()). One outside cachemin/cachemax is kept as an empty file,
// which tells the readers it is not cached. The file was renamed first and emptied after:
// a reader process that opened and mapped it in between got SIGBUS on the pages that went
// away.
//
// The window is two system calls wide. A preloaded rename() looks at the file right after it
// got its final name, which is where another reader can find it: an article larger than
// cachemax must have no bytes there already.
func TestDeepRevCacheTruncateBeforeRename(t *testing.T) {
	so := preloadShim(t, "cacherename", withMark(cacheRenameSource))
	mark := filepath.Join(t.TempDir(), "mark")
	st, spool := spoolStore(t)
	s := NewReaderSite(t, ReaderOptions{Spool: spool, NoStart: true, Config: map[string]string{"readercache": "on"}})
	s.WriteFile(t, "dserver.hosts", "127.0.0.1\ts1p"+strings.TrimPrefix(spool.Addr(), "127.0.0.1:")+"\tcache=on cachemax=10\n")
	s.Env = append(s.Env, lockFaultEnv(so), "SHIM_MARK="+mark, "CACHERENAME_DIR="+filepath.Join(s.Dir, "spool", "cache"))
	t.Cleanup(func() {
		if t.Failed() {
			t.Logf("general.log:\n%s\nmarks:\n%s", generalLog(s), strings.Join(shimMarks(mark), "\n"))
		}
	})
	startReader(t, s, "[127.0.0.1]:"+strings.TrimPrefix(s.Addr, "127.0.0.1:"))
	feedReader(t, s, st, "e2e.test", newArticle(msgID(t), "e2e.test", "larger than cachemax", "a body of more than ten bytes"))

	for i := 1; i <= 2; i++ {
		if ok, d := readArticleOnce(t, s, "e2e.test", 1); !ok {
			t.Fatalf("read %d: %s", i, d)
		}
	}
	var installed []string
	testkit.WaitFor(t, 10*time.Second, "the cache file to be installed", func() bool {
		installed = shimMarked(mark, "installed ")
		return len(installed) > 0
	})
	sizeRe := regexp.MustCompile(` size (\d+)$`)
	for _, l := range installed {
		m := sizeRe.FindStringSubmatch(l)
		if m == nil {
			t.Fatalf("mark %q", l)
		}
		if n, _ := strconv.Atoi(m[1]); n != 0 {
			t.Errorf("the cache file of an article larger than cachemax=10 got its final name with %d bytes, emptied only after the rename (%s)", n, l)
		}
	}
}

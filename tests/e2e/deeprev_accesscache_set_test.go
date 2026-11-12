package e2e

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"

	testkit "github.com/go-while/nntp-testkit"
)

// accessFaultSource makes file calls of the access cache install fail with EIO while the
// file ACFAULT_FLAG exists: rename() of a name ending in one of the comma-separated suffixes
// in ACFAULT_RENAME, unlink() of one in ACFAULT_UNLINK; link() from one in ACFAULT_LINK while
// the file ACFAULT_LINKFLAG exists (ACFAULT_FLAG when unset); fstat() of a descriptor whose
// file name ends in one in ACFAULT_FSTAT while ACFAULT_FSTATFLAG exists. Each refused call is
// marked.
const accessFaultSource = `#define _GNU_SOURCE
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
/*FDPATH*/
/*MARK*/
static int
ends(const char *path, const char *list)
{
    size_t n = strlen(path);

    while (list != NULL && *list) {
	const char *e = strchr(list, ',');
	size_t l = e ? (size_t)(e - list) : strlen(list);

	if (l > 0 && n >= l && strncmp(path + n - l, list, l) == 0)
	    return 1;
	list = e ? e + 1 : NULL;
    }
    return 0;
}

static int
active(const char *env, const char *path, const char *flagenv)
{
    const char *flag = getenv(flagenv);

    if (flag == NULL)
	flag = getenv("ACFAULT_FLAG");
    return flag != NULL && ends(path, getenv(env)) && access(flag, F_OK) == 0;
}

int
rename(const char *from, const char *to)
{
    static int (*real)(const char *, const char *);

    if (real == NULL)
	real = (int (*)(const char *, const char *))dlsym(RTLD_NEXT, "rename");
    if (active("ACFAULT_RENAME", from, "ACFAULT_FLAG")) {
	shim_mark("refused rename of %s", from);
	errno = EIO;
	return -1;
    }
    return real(from, to);
}

int
link(const char *from, const char *to)
{
    static int (*real)(const char *, const char *);

    if (real == NULL)
	real = (int (*)(const char *, const char *))dlsym(RTLD_NEXT, "link");
    if (active("ACFAULT_LINK", from, "ACFAULT_LINKFLAG")) {
	shim_mark("refused link of %s", from);
	errno = getenv("ACFAULT_LINK_EPERM") != NULL ? EPERM : EIO;
	return -1;
    }
    return real(from, to);
}

int
unlink(const char *path)
{
    static int (*real)(const char *);

    if (real == NULL)
	real = (int (*)(const char *))dlsym(RTLD_NEXT, "unlink");
    if (active("ACFAULT_UNLINK", path, "ACFAULT_FLAG")) {
	shim_mark("refused unlink of %s", path);
	errno = EIO;
	return -1;
    }
    return real(path);
}

static int
fstat_refused(int fd)
{
    char path[4096];

    if (getenv("ACFAULT_FSTAT") == NULL)
	return 0;
    fdpath(fd, path, sizeof(path));
    if (path[0] && active("ACFAULT_FSTAT", path, "ACFAULT_FSTATFLAG")) {
	shim_mark("refused fstat of %s", path);
	errno = EIO;
	return 1;
    }
    return 0;
}

/*
 * NetBSD's <sys/stat.h> renames fstat() to __fstat50 (the struct stat of today), so this
 * definition is __fstat50 there, and the libc symbol "fstat" is the old one for old binaries
 * with another struct stat: look up the same name. Not handled: 32-bit musl with 64-bit
 * time_t (_REDIR_TIME64), where "fstat" may name an old-layout symbol (the lookup then does
 * not fail).
 */
#ifdef __NetBSD__
#define FSTAT_NAME "__fstat50"
#else
#define FSTAT_NAME "fstat"
#endif

int
fstat(int fd, struct stat *st)
{
    static int (*real)(int, struct stat *);

    if (real == NULL)
	real = (int (*)(int, struct stat *))dlsym(RTLD_NEXT, FSTAT_NAME);
    if (real == NULL) {
	shim_mark("dlsym(%s) failed", FSTAT_NAME);
	abort();
    }
    if (fstat_refused(fd))
	return -1;
    return real(fd, st);
}

#ifdef __GLIBC__	/* with _FILE_OFFSET_BITS=64, and before glibc 2.33 */
int
fstat64(int fd, struct stat64 *st)
{
    static int (*real)(int, struct stat64 *);

    if (real == NULL)
	real = (int (*)(int, struct stat64 *))dlsym(RTLD_NEXT, "fstat64");
    if (fstat_refused(fd))
	return -1;
    return real(fd, st);
}

int
__fxstat64(int ver, int fd, struct stat64 *st)
{
    static int (*real)(int, int, struct stat64 *);

    if (real == NULL)
	real = (int (*)(int, int, struct stat64 *))dlsym(RTLD_NEXT, "__fxstat64");
    if (fstat_refused(fd))
	return -1;
    return real(ver, fd, st);
}
#endif
`

// accessFaultSite starts a reader with dreader.access a (127.0.0.1 gets the group list
// "testonly", e2e.test) and the access cache fault injector with the given environment, and
// returns the site, the fault flag file, the mark file and LIST ACTIVE as a function.
func accessFaultSite(t *testing.T, env ...string) (*Site, string, string, func() []string) {
	t.Helper()
	so := preloadShim(t, "accessfault", withMark(withFdPath(accessFaultSource)))
	dir := t.TempDir()
	flag := filepath.Join(dir, "fail")
	mark := filepath.Join(dir, "mark")
	_, spool := spoolStore(t)
	s := NewReaderSite(t, ReaderOptions{Spool: spool, Access: accessSetA, NoStart: true})
	s.Env = append(s.Env, lockFaultEnv(so), "SHIM_MARK="+mark, "ACFAULT_FLAG="+flag)
	s.Env = append(s.Env, env...)
	t.Cleanup(func() {
		if t.Failed() {
			t.Logf("general.log:\n%s\nmarks:\n%s", generalLog(s), strings.Join(shimMarks(mark), "\n"))
		}
	})
	startReader(t, s, "[127.0.0.1]:"+strings.TrimPrefix(s.Addr, "127.0.0.1:"))
	list := func() []string {
		t.Helper()
		c := s.Reader(t)
		defer c.Close()
		c.ExpectCmd(215, "LIST ACTIVE")
		names := activeNames(c.ReadDotLines())
		c.ExpectCmd(205, "QUIT")
		return names
	}
	if got := list(); !slices.Equal(got, []string{"e2e.test"}) {
		t.Fatalf("with dreader.access A: groups %q, want [e2e.test]", got)
	}
	return s, flag, mark, list
}

// accessSet returns a dreader.access that gives 127.0.0.1 the group list groups; accessSetA
// gives "testonly" and has no group list "otheronly", accessSetB gives the new "otheronly".
func accessSet(groups string, other bool) string {
	a := `vserverdef DEFAULT
    hostname ` + PathHost + `
end
groupdef testonly
    group e2e.test
end
`
	if other {
		a += "groupdef otheronly\n    group e2e.other\nend\n"
	}
	return a + `readerdef rp
    read yes
    groups ` + groups + `
end
access 127.0.0.1 rp
`
}

var (
	accessSetA = accessSet("testonly", false)
	accessSetB = accessSet("otheronly", true)
)

var accessCacheFiles = []string{"cache.vserverdef", "cache.groupdef", "cache.authdef", "cache.readerdef", "cache.readeraccess"}

// fileIds returns "inode/size/mtime" of each file (or "missing").
func fileIds(t *testing.T, dir string, names ...string) map[string]string {
	t.Helper()
	m := map[string]string{}
	for _, n := range names {
		fi, err := os.Stat(filepath.Join(dir, n))
		if err != nil {
			m[n] = "missing"
			continue
		}
		st := fi.Sys().(*syscall.Stat_t)
		m[n] = fmt.Sprintf("%d/%d/%d", st.Ino, fi.Size(), fi.ModTime().UnixNano())
	}
	return m
}

// waitB lifts the faults and waits for dreader.access B to be in use (e2e.other only).
func waitB(t *testing.T, flag string, list func() []string, extra ...string) {
	t.Helper()
	for _, f := range append([]string{flag}, extra...) {
		if err := os.Remove(f); err != nil && !os.IsNotExist(err) {
			t.Fatal(err)
		}
	}
	var got []string
	testkit.WaitFor(t, 15*time.Second, "B to be installed once the faults are gone", func() bool {
		got = list()
		return slices.Equal(got, []string{"e2e.other"})
	})
}

// TestDeepRevAccessCacheSetKept (L6-7, second review): the five access cache files are one
// set; RenameAccessCache() renames the new ones over the old ones. When one rename failed it
// renamed the others anyway and wrote the lock stamp, so the readers loaded new files next to
// an old one: a new readerdef with an old cache.groupdef that does not have its group list
// gets the first list, every group.
//
// dreader.access A gives 127.0.0.1 the group list "testonly" (e2e.test). It is replaced by B,
// which gives the new list "otheronly" (e2e.other), while a preloaded rename() refuses
// cache.groupdef.new. The client must still get A's group, not every group; once the rename
// works again, B's.
func TestDeepRevAccessCacheSetKept(t *testing.T) {
	s, flag, mark, list := accessFaultSite(t, "ACFAULT_RENAME=/cache.groupdef.new")
	if err := os.WriteFile(flag, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	s.WriteFile(t, "dreader.access", accessSetB)
	testkit.WaitFor(t, 15*time.Second, "the install of B with the rename of cache.groupdef refused", func() bool {
		return len(shimMarked(mark, "refused rename")) > 0 &&
			strings.Contains(generalLog(s), "Unable to rename ")
	})
	time.Sleep(time.Second)
	for i := 0; i < 3; i++ { // the reader and the DNS worker both load the cache
		if got := list(); !slices.Equal(got, []string{"e2e.test"}) {
			t.Fatalf("after the install of B whose cache.groupdef could not be renamed: groups %q, want A's [e2e.test]: "+
				"the readers loaded new cache files next to the old cache.groupdef", got)
		}
	}
	waitB(t, flag, list)
	for _, f := range accessCacheFiles {
		if _, err := os.Stat(filepath.Join(s.Dir, f+".old")); err == nil {
			t.Errorf("%s.old left behind", f)
		}
	}
}

// TestDeepRevAccessCacheLinkNotSupported: every install after the first keeps a hard link
// to each access cache file in use. On a file system without hard links (FAT, some SMB and
// FUSE mounts: link() fails with EPERM) no later install can work; the log must say so,
// rather than only "Operation not permitted".
//
// A preloaded link() fails with EPERM for cache.readerdef: the log names the cause, the client
// keeps A; once link() works again, B is installed.
func TestDeepRevAccessCacheLinkNotSupported(t *testing.T) {
	s, flag, mark, list := accessFaultSite(t, "ACFAULT_LINK=/cache.readerdef", "ACFAULT_LINK_EPERM=1")
	if err := os.WriteFile(flag, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	s.WriteFile(t, "dreader.access", accessSetB)
	testkit.WaitFor(t, 15*time.Second, "the install of B with the link of cache.readerdef refused", func() bool {
		return len(shimMarked(mark, "refused link")) > 0
	})
	testkit.WaitFor(t, 5*time.Second, "the refused link in the log", func() bool {
		return strings.Contains(generalLog(s), "Unable to link ")
	})
	if want := "the db directory must be on a file system with hard links"; !strings.Contains(generalLog(s), want) {
		t.Errorf("the log of a link() refused with EPERM does not say %q", want)
	}
	if got := list(); !slices.Equal(got, []string{"e2e.test"}) {
		t.Errorf("after the failed install: groups %q, want A's [e2e.test]", got)
	}
	waitB(t, flag, list)
}

// TestDeepRevAccessCacheLinkFailure (third review): before the renames each access cache file
// in use gets a hard link <name>.old, to put it back when a later rename fails. A link that
// failed for any reason was taken as "there was no such file", and the put-back then removed
// that file: after a failed install the name had no file at all, and a process loading the
// cache found the set incomplete. Only a missing file needs no link; any other failure must
// stop the install before the first rename.
//
// A preloaded link() fails for cache.readerdef (EIO) and rename() for cache.readeraccess.new:
// no access cache file may change, the lock file keeps its stamp, the client keeps A; once
// the faults are gone, B is installed.
func TestDeepRevAccessCacheLinkFailure(t *testing.T) {
	s, flag, mark, list := accessFaultSite(t, "ACFAULT_LINK=/cache.readerdef", "ACFAULT_RENAME=/cache.readeraccess.new")
	files := append(slices.Clone(accessCacheFiles), ".accesslock")
	before := fileIds(t, s.Dir, files...)
	if err := os.WriteFile(flag, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	s.WriteFile(t, "dreader.access", accessSetB)
	testkit.WaitFor(t, 15*time.Second, "the install of B with the link of cache.readerdef refused", func() bool {
		return len(shimMarked(mark, "refused link")) > 0
	})
	time.Sleep(time.Second)
	after := fileIds(t, s.Dir, files...)
	for _, f := range files {
		if after[f] != before[f] {
			t.Errorf("%s changed in an install that could not keep a link to cache.readerdef: %s, was %s", f, after[f], before[f])
		}
	}
	if !strings.Contains(generalLog(s), "Unable to link ") {
		t.Errorf("the refused link is not in the log")
	}
	if strings.Contains(generalLog(s), "file system with hard links") {
		t.Errorf("a link() that failed with EIO is logged as a file system without hard links")
	}
	if got := list(); !slices.Equal(got, []string{"e2e.test"}) {
		t.Errorf("after the failed install: groups %q, want A's [e2e.test]", got)
	}
	waitB(t, flag, list)
}

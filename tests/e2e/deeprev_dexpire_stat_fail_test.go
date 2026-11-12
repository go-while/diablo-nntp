package e2e

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

// statFailSource makes stat() of a path that holds STATFAIL_PATH fail with EIO (with
// STATFAIL_COUNT=n only the first n times), and appends the path to the file STATFAIL_MARK.
// On NetBSD <sys/stat.h> renames stat() to __stat50, so the stat() defined here is __stat50
// too, but the real one must be looked up by that name: "stat" is the old-ABI compat
// function, whose struct stat does not match (the D-170 select() shim had the same problem).
const statFailSource = `#define _GNU_SOURCE
#define __EXTENSIONS__
#if defined(__NetBSD__)
#define REAL_STAT "__stat50"	/* <sys/stat.h> renames stat() to it; "stat" is the old ABI */
#else
#define REAL_STAT "stat"
#endif
#include <dlfcn.h>
#include <errno.h>
#include <fcntl.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/stat.h>
#include <unistd.h>
/*FDPATH*/
static int
fail(const char *path)
{
    static int hits;
    const char *only = getenv("STATFAIL_PATH");
    const char *mark = getenv("STATFAIL_MARK");
    const char *count = getenv("STATFAIL_COUNT");
    FILE *f;

    if (only == NULL || strstr(path, only) == NULL)
	return 0;
    if (count != NULL && hits >= atoi(count))
	return 0;
    ++hits;
    if (mark != NULL && (f = fopen(mark, "a")) != NULL) {
	fprintf(f, "%s\n", path);
	fclose(f);
    }
    errno = EIO;
    return 1;
}

int
stat(const char *path, struct stat *st)
{
    static int (*real)(const char *, struct stat *);

    if (real == NULL)
	real = (int (*)(const char *, struct stat *))dlsym(RTLD_NEXT, REAL_STAT);
    if (fail(path))
	return -1;
    return real(path, st);
}

#ifdef __GLIBC__	/* the name stat() has with _FILE_OFFSET_BITS=64 */
int
stat64(const char *path, struct stat64 *st)
{
    static int (*real)(const char *, struct stat64 *);

    if (real == NULL)
	real = (int (*)(const char *, struct stat64 *))dlsym(RTLD_NEXT, "stat64");
    if (fail(path))
	return -1;
    return real(path, st);
}
#endif
`

// TestDeepRevDexpireStatFail (Terra T4, second review of SOL-2): dexpire takes a D.* entry as a
// spool directory only when stat() says it is one. A failed stat() (an I/O or permission error,
// not a missing entry) skipped the entry as well, but it may be a real directory: the history
// update, which keeps only the articles of the directories in the list, then marked its
// articles expired although they are still on disk.
//
// Such an entry now stays in the list, and nothing of its spool is removed in that run. The test
// stores an article, adds a newer directory, makes stat() of the article's directory fail with
// a preloaded library and runs dexpire -a -h (history update): the article must stay readable
// (STAT 223), both directories stay, and dexpire says why.
func TestDeepRevDexpireStatFail(t *testing.T) {
	so := preloadShim(t, "statfail", statFailSource)
	s := NewSite(t, SiteOptions{SpoolCtl: "spool 00\n    minfree 1m\n    keeptime 1\nend\nmetaspool trad\n    spool 00\nend\nexpire * trad\n"})
	id := s.Inject(t, "e2e.test", 1)[0]
	dirs, _ := filepath.Glob(filepath.Join(s.Dir, "spool", "news", "D.*"))
	if len(dirs) != 1 {
		t.Fatalf("spool directories: %q, want one", dirs)
	}
	time.Sleep(2 * time.Second) // past keeptime 1 (seconds)
	newerSpoolDir(t, s)
	all, _ := filepath.Glob(filepath.Join(s.Dir, "spool", "news", "D.*"))

	mark := filepath.Join(t.TempDir(), "stat")
	base := s.Env
	s.Env = append(slices.Clone(base), lockFaultEnv(so), "STATFAIL_PATH="+filepath.Base(dirs[0]), "STATFAIL_MARK="+mark)
	out, err := s.TryRun("dexpire", "-a", "-h", "-v")
	s.Env = base
	if b, err := os.ReadFile(mark); err != nil || len(b) == 0 {
		t.Fatalf("the fault was never hit (%v):\n%s", err, out)
	}
	if err != nil {
		t.Errorf("dexpire -a -h: %v", err)
	}
	for _, d := range all {
		if _, err := os.Stat(d); err != nil {
			t.Errorf("%s is gone: %v", d, err)
		}
	}
	if !strings.Contains(out, "cannot tell what spool entry") {
		t.Errorf("dexpire did not report the entry it could not stat")
	}
	p := s.Peer(t)
	if code, line := p.Cmd("STAT %s", id); code != 223 {
		t.Errorf("STAT of the article in the directory dexpire could not stat: %q, want 223 (its history entry was expired)", line)
	}
	p.ExpectCmd(205, "QUIT")
	if t.Failed() {
		t.Logf("dexpire -a -h -v: err=%v\n%s", err, out)
	}
}

// TestDeepRevDexpireStatFailOnce (Sol R4, re-review): dexpire makes its partitions again for
// every pass over the spools, and a second pass comes when the first removed something. The
// mark "nothing removed from this spool in this run" was lost with the first pass: when stat()
// worked again, the second pass removed directories of that spool.
//
// Spools 00 and 01 with keeptime 1 s, each with a directory 30 minutes old and one 5 minutes
// old. stat() of the old directory of spool 00 fails once (in the first pass). The old one of
// spool 01 goes in the first pass, so there is a second; the old one of spool 00 must stay.
func TestDeepRevDexpireStatFailOnce(t *testing.T) {
	so := preloadShim(t, "statfail", statFailSource)
	now := time.Now().Unix() / 60
	var oldA, newA, oldB string
	s := NewSite(t, SiteOptions{
		NoStart: true,
		SpoolCtl: `spool 00
    keeptime 1
end
spool 01
    keeptime 1
end
metaspool m
    spool 00
    spool 01
end
expire * m
`,
		Setup: func(tb testing.TB, s *Site) {
			top := filepath.Join(s.Dir, "spool", "news")
			oldA = filepath.Join(top, fmt.Sprintf("D.%08x", now-30))
			newA = filepath.Join(top, fmt.Sprintf("D.%08x", now-5))
			oldB = filepath.Join(top, "P.01", fmt.Sprintf("D.%08x", now-31))
			newB := filepath.Join(top, "P.01", fmt.Sprintf("D.%08x", now-6))
			for _, d := range []string{oldA, newA, oldB, newB} {
				if err := os.MkdirAll(d, 0o755); err != nil {
					tb.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(d, "B.0001"), []byte("article\n"), 0o644); err != nil {
					tb.Fatal(err)
				}
			}
		},
	})
	mark := filepath.Join(t.TempDir(), "stat")
	base := s.Env
	s.Env = append(slices.Clone(base), lockFaultEnv(so), "STATFAIL_PATH="+filepath.Base(oldA), "STATFAIL_COUNT=1", "STATFAIL_MARK="+mark)
	out, err := s.TryRun("dexpire", "-a", "-h0", "-v")
	s.Env = base
	if b, err := os.ReadFile(mark); err != nil || len(b) == 0 {
		t.Fatalf("the fault was never hit (%v):\n%s", err, out)
	}
	if err != nil {
		t.Errorf("dexpire -a -h0: %v", err)
	}
	if _, err := os.Stat(oldB); err == nil {
		t.Fatalf("test setup: the old directory of spool 01 was not removed, so there is no second pass:\n%s", out)
	}
	if !strings.Contains(out, "Spool scan pass 2") {
		t.Fatalf("test setup: no second pass:\n%s", out)
	}
	if _, err := os.Stat(oldA); err != nil {
		t.Errorf("the second pass removed %s, of the spool the first pass left alone: %v\n%s", oldA, err, out)
	}
}

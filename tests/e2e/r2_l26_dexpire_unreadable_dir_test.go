package e2e

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// dirFailSource makes opendir() (DIRFAIL_MODE=open) or readdir() (DIRFAIL_MODE=read) of the
// directory whose last path component is DIRFAIL_NAME fail with EIO, and appends the path to
// the file DIRFAIL_MARK. On NetBSD <dirent.h> renames opendir() and readdir() to __opendir30
// and __readdir30; the real ones must be looked up by those names (see statFailSource).
const dirFailSource = `#define _GNU_SOURCE
#define __EXTENSIONS__
#if defined(__NetBSD__)
#define REAL_OPENDIR "__opendir30"
#define REAL_READDIR "__readdir30"
#else
#define REAL_OPENDIR "opendir"
#define REAL_READDIR "readdir"
#endif
#include <dirent.h>
#include <dlfcn.h>
#include <errno.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
static DIR *faildir;	/* the open directory whose readdir() fails */

static int
mode(const char *m)
{
    const char *e = getenv("DIRFAIL_MODE");

    return e != NULL && strcmp(e, m) == 0;
}

static int
match(const char *path)
{
    const char *name = getenv("DIRFAIL_NAME");
    const char *mark = getenv("DIRFAIL_MARK");
    size_t l = strlen(path);
    size_t nl;
    FILE *f;

    if (name == NULL)
	return 0;
    while (l > 1 && path[l - 1] == '/')
	--l;
    nl = strlen(name);
    if (l < nl || strncmp(path + l - nl, name, nl) != 0 ||
				(l > nl && path[l - nl - 1] != '/'))
	return 0;
    if (mark != NULL && (f = fopen(mark, "a")) != NULL) {
	fprintf(f, "%s\n", path);
	fclose(f);
    }
    return 1;
}

DIR *
opendir(const char *path)
{
    static DIR *(*real)(const char *);
    DIR *d;
    int hit;

    if (real == NULL)
	real = (DIR *(*)(const char *))dlsym(RTLD_NEXT, REAL_OPENDIR);
    hit = match(path);
    if (hit && mode("open")) {
	errno = EIO;
	return NULL;
    }
    d = real(path);
    if (hit && d != NULL && mode("read"))
	faildir = d;
    return d;
}

struct dirent *
readdir(DIR *d)
{
    static struct dirent *(*real)(DIR *);

    if (real == NULL)
	real = (struct dirent *(*)(DIR *))dlsym(RTLD_NEXT, REAL_READDIR);
    if (d != NULL && d == faildir) {
	errno = EIO;
	return NULL;
    }
    return real(d);
}

#ifdef __GLIBC__	/* the name readdir() has with _FILE_OFFSET_BITS=64 */
struct dirent64 *
readdir64(DIR *d)
{
    static struct dirent64 *(*real)(DIR *);

    if (real == NULL)
	real = (struct dirent64 *(*)(DIR *))dlsym(RTLD_NEXT, "readdir64");
    if (d != NULL && d == faildir) {
	errno = EIO;
	return NULL;
    }
    return real(d);
}
#endif

int
closedir(DIR *d)
{
    static int (*real)(DIR *);

    if (real == NULL)
	real = (int (*)(DIR *))dlsym(RTLD_NEXT, "closedir");
    if (d == faildir)
	faildir = NULL;
    return real(d);
}
`

// TestR2DexpireUnreadableSpoolDir (deep review r2 L26, D-268): dexpire keeps the history
// records of the spool directories it has seen and marks every other record expired. A spool
// directory that opendir() or readdir() could not read counted as an empty one, so every
// article stored below it was marked expired and the feeder answered 430 for articles that
// were still on disk.
//
// Such a directory now leaves the records below it as they are for that run (and a readdir()
// error removes nothing from it). Spools 00 and 01, each with its own group: the directory of
// one spool cannot be read (opendir() of P.01 fails, or readdir() of the spool home fails),
// its two articles must still answer STAT 223 after dexpire -a -h1. The other spool shows that
// the rest of the history is still updated: the directory of its one article was removed by
// hand, so that record must be marked expired (430).
func TestR2DexpireUnreadableSpoolDir(t *testing.T) {
	const spoolCtl = `spool 00
    minfree 1m
end
spool 01
    minfree 1m
end
metaspool m0
    spool 00
end
metaspool m1
    spool 01
end
expire e2e.zero m0
expire e2e.one m1
`
	so := preloadShim(t, "dirfail", dirFailSource)
	for _, tc := range []struct {
		name     string
		mode     string // DIRFAIL_MODE
		fail     string // DIRFAIL_NAME: the spool directory that cannot be read
		keep     string // its group
		gone     string // group of the other spool
		goneDirs string // glob of the other spool's directories, below spool/news
	}{
		{"opendir", "open", "P.01", "e2e.one", "e2e.zero", "D.*"},
		{"readdir", "read", "news", "e2e.zero", "e2e.one", "P.01/D.*"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := NewSite(t, SiteOptions{
				SpoolCtl: spoolCtl,
				Setup: func(tb testing.TB, s *Site) {
					if err := os.MkdirAll(filepath.Join(s.Dir, "spool", "news", "P.01"), 0o755); err != nil {
						tb.Fatal(err)
					}
				},
			})
			keep := s.Inject(t, tc.keep, 2)
			gone := s.Inject(t, tc.gone, 1)[0]
			dirs, _ := filepath.Glob(filepath.Join(s.Dir, "spool", "news", tc.goneDirs))
			if len(dirs) != 1 {
				t.Fatalf("test setup: directories of the article of %s: %q, want one", tc.gone, dirs)
			}
			if err := os.RemoveAll(dirs[0]); err != nil {
				t.Fatal(err)
			}

			mark := filepath.Join(t.TempDir(), "dirfail")
			base := s.Env
			s.Env = append(slices.Clone(base), lockFaultEnv(so), "DIRFAIL_MODE="+tc.mode, "DIRFAIL_NAME="+tc.fail, "DIRFAIL_MARK="+mark)
			out, err := s.TryRun("dexpire", "-a", "-h1", "-v")
			s.Env = base
			if b, err := os.ReadFile(mark); err != nil || len(b) == 0 {
				t.Fatalf("the fault was never hit (%v):\n%s", err, out)
			}
			if err != nil {
				t.Errorf("dexpire -a -h1: %v", err)
			}
			if !strings.Contains(out, "not updated in this run") {
				t.Errorf("dexpire did not report the spool directory it could not read")
			}

			p := s.Peer(t)
			for _, id := range keep {
				if code, line := p.Cmd("STAT %s", id); code != 223 {
					t.Errorf("STAT %s (in the directory dexpire could not read): %q, want 223 (its history record was marked expired)", id, line)
				}
			}
			if code, line := p.Cmd("STAT %s", gone); code != 430 {
				t.Errorf("STAT %s (its directory was removed): %q, want 430 (the history of the other spool must still be updated)", gone, line)
			}
			p.ExpectCmd(205, "QUIT")
			if t.Failed() {
				t.Logf("dexpire -a -h1 -v: err=%v\n%s", err, out)
			}
		})
	}
}

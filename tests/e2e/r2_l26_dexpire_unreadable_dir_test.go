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

// dirFailSource makes opendir() (DIRFAIL_MODE=open) or readdir() (DIRFAIL_MODE=read) of the
// directory whose last path component is DIRFAIL_NAME fail, with EIO, or with ENOENT when
// DIRFAIL_ERRNO=ENOENT, and appends the path to the file DIRFAIL_MARK. readdir() fails at
// once, after DIRFAIL_AFTER=n entries, or with DIRFAIL_AFTER=end instead of the end of the
// directory (after every entry). On NetBSD <dirent.h> renames opendir() and readdir() to
// __opendir30 and __readdir30; the real ones must be looked up by those names (see
// statFailSource).
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
static long entries;	/* returned from it so far */

static int
mode(const char *m)
{
    const char *e = getenv("DIRFAIL_MODE");

    return e != NULL && strcmp(e, m) == 0;
}

static int
failerrno(void)
{
    const char *e = getenv("DIRFAIL_ERRNO");

    return (e != NULL && strcmp(e, "ENOENT") == 0) ? ENOENT : EIO;
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
	errno = failerrno();
	return NULL;
    }
    d = real(path);
    if (hit && d != NULL && mode("read")) {
	faildir = d;
	entries = 0;
    }
    return d;
}

/* whether this readdir() of d fails now; got is what the real one returned */
static int
readfail(DIR *d, const void *got)
{
    const char *after = getenv("DIRFAIL_AFTER");

    if (d == NULL || d != faildir)
	return 0;
    if (after != NULL && strcmp(after, "end") == 0) {
	if (got != NULL)
	    return 0;
    } else if (after != NULL && entries < atol(after)) {
	if (got != NULL)
	    ++entries;
	return 0;
    }
    errno = failerrno();
    return 1;
}

struct dirent *
readdir(DIR *d)
{
    static struct dirent *(*real)(DIR *);
    struct dirent *e;

    if (real == NULL)
	real = (struct dirent *(*)(DIR *))dlsym(RTLD_NEXT, REAL_READDIR);
    e = real(d);
    return readfail(d, e) ? NULL : e;
}

#ifdef __GLIBC__	/* the name readdir() has with _FILE_OFFSET_BITS=64 */
struct dirent64 *
readdir64(DIR *d)
{
    static struct dirent64 *(*real)(DIR *);
    struct dirent64 *e;

    if (real == NULL)
	real = (struct dirent64 *(*)(DIR *))dlsym(RTLD_NEXT, "readdir64");
    e = real(d);
    return readfail(d, e) ? NULL : e;
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

// dirFailEnv is the environment for a dexpire run with the dirFailSource library so: fail is
// DIRFAIL_NAME, extra further DIRFAIL_* settings; mark is the file the library writes to.
func dirFailEnv(base []string, so, mark, mode, fail string, extra ...string) []string {
	env := append(slices.Clone(base), lockFaultEnv(so), "DIRFAIL_MODE="+mode, "DIRFAIL_NAME="+fail, "DIRFAIL_MARK="+mark)
	return append(env, extra...)
}

// TestR2DexpireUnreadableSpoolDir (deep review r2 L26, D-268): dexpire keeps the history
// records of the spool directories it has seen and marks every other record expired. A spool
// directory that opendir() or readdir() could not read counted as an empty one, so every
// article stored below it was marked expired and the feeder answered 430 for articles that
// were still on disk.
//
// Such a directory now leaves the records below it as they are for that run. Spools 00 and 01,
// each with its own group: the directory of one spool cannot be read (opendir() of P.01 fails,
// or readdir() of the spool home fails at once or after two entries), its two articles must
// still answer STAT 223 after dexpire -a -h1, with valid history records. The other spool shows
// that the rest of the history is still updated: the directory of its one article was removed
// by hand, so that record must be marked expired (dilookup: EX=(expired); STAT cannot tell, the
// feeder answers 430 for a record whose file is gone, expired or not).
//
// A directory that is gone (ENOENT) is empty, as before: its records are marked expired.
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
		mode     string   // DIRFAIL_MODE
		fail     string   // DIRFAIL_NAME: the spool directory that cannot be read
		extra    []string // further DIRFAIL_* settings
		keep     string   // its group
		gone     string   // group of the other spool
		goneDirs string   // glob of the other spool's directories, below spool/news
		expired  bool     // the records below the failing directory are expired (ENOENT)
	}{
		{"opendir", "open", "P.01", nil, "e2e.one", "e2e.zero", "D.*", false},
		{"readdir", "read", "news", nil, "e2e.zero", "e2e.one", "P.01/D.*", false},
		{"readdir_partial", "read", "news", []string{"DIRFAIL_AFTER=2"}, "e2e.zero", "e2e.one", "P.01/D.*", false},
		{"opendir_enoent", "open", "P.01", []string{"DIRFAIL_ERRNO=ENOENT"}, "e2e.one", "e2e.zero", "D.*", true},
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
			s.Env = dirFailEnv(base, so, mark, tc.mode, tc.fail, tc.extra...)
			out, err := s.TryRun("dexpire", "-a", "-h1", "-v")
			s.Env = base
			if b, err := os.ReadFile(mark); err != nil || len(b) == 0 {
				t.Fatalf("the fault was never hit (%v):\n%s", err, out)
			}
			if err != nil {
				t.Errorf("dexpire -a -h1: %v", err)
			}
			if reported := strings.Contains(out, "not updated in this run"); reported == tc.expired {
				t.Errorf("dexpire reported records not updated: %v, want %v", reported, !tc.expired)
			}

			wantCode, wantEx := 223, "valid"
			if tc.expired {
				wantCode, wantEx = 430, "expired"
			}
			p := s.Peer(t)
			for _, id := range keep {
				if code, line := p.Cmd("STAT %s", id); code != wantCode {
					t.Errorf("STAT %s (below the directory dexpire could not read): %q, want %d", id, line, wantCode)
				}
			}
			p.ExpectCmd(205, "QUIT")
			for _, id := range keep {
				if ex := r2l26HistoryEx(t, s, id); ex != wantEx {
					t.Errorf("history record of %s (below the directory dexpire could not read): EX=(%s), want %s", id, ex, wantEx)
				}
			}
			if ex := r2l26HistoryEx(t, s, gone); ex != "expired" {
				t.Errorf("history record of %s (its directory was removed): EX=(%s), want expired (the history of the other spool must still be updated)", gone, ex)
			}
			if t.Failed() {
				t.Logf("dexpire -a -h1 -v: err=%v\n%s", err, out)
			}
		})
	}
}

// TestR2DexpireReadErrorNoRemoval (deep review r2 L26): after a readdir() error the list of a
// spool directory may lack entries, so dexpire cannot tell which directory is the newest, and
// the history update leaves the records below it alone. Nothing may then be removed from it,
// nor from the N.xx directories below it that were listed before the error (their records
// are left alone as well).
//
// Spool 01 with maxsize 1m holds a 1000 KB directory an hour old and a newer one of 100 KB,
// directly (case "spool") or in P.01/N.00 with spooldirs 2 (case "spooldirs"). readdir() of
// P.01 fails where the end of the directory would be, so every entry was listed: without the
// rule the old directory would be removed for maxsize. It must still be there.
func TestR2DexpireReadErrorNoRemoval(t *testing.T) {
	so := preloadShim(t, "dirfail", dirFailSource)
	now := time.Now().Unix() / 60
	for _, tc := range []struct {
		name string
		dirs string // spooldirs line
		sub  string // where the directories are, below P.01
	}{
		{"spool", "", ""},
		{"spooldirs", "    spooldirs 2\n", "N.00"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			part := filepath.Join("spool", "news", "P.01", tc.sub)
			old := fmt.Sprintf("D.%08x", now-60)
			newer := fmt.Sprintf("D.%08x", now-5)
			s := NewSite(t, SiteOptions{
				NoStart:  true,
				SpoolCtl: "spool 01\n    maxsize 1m\n" + tc.dirs + "end\nmetaspool m\n    spool 01\nend\nexpire * m\n",
				Setup: func(tb testing.TB, s *Site) {
					for _, d := range []struct {
						name string
						size int
					}{{old, 1000 * 1024}, {newer, 100 * 1024}} {
						dir := filepath.Join(s.Dir, part, d.name)
						if err := os.MkdirAll(dir, 0o755); err != nil {
							tb.Fatal(err)
						}
						if err := os.WriteFile(filepath.Join(dir, "B.0001"), make([]byte, d.size), 0o644); err != nil {
							tb.Fatal(err)
						}
					}
				},
			})
			mark := filepath.Join(t.TempDir(), "dirfail")
			base := s.Env
			s.Env = dirFailEnv(base, so, mark, "read", "P.01", "DIRFAIL_AFTER=end")
			out, err := s.TryRun("dexpire", "-a", "-h0", "-v")
			s.Env = base
			if b, err := os.ReadFile(mark); err != nil || len(b) == 0 {
				t.Fatalf("the fault was never hit (%v):\n%s", err, out)
			}
			if err != nil {
				t.Errorf("dexpire -a -h0: %v", err)
			}
			if _, err := os.Stat(filepath.Join(s.Dir, part, old)); err != nil {
				t.Errorf("%s was removed after a readdir() error of P.01: %v\n%s", filepath.Join(part, old), err, out)
			}

			// The same spool without the fault: the old directory goes (the test can tell).
			out = s.Run(t, "dexpire", "-a", "-h0", "-v")
			if _, err := os.Stat(filepath.Join(s.Dir, part, old)); err == nil {
				t.Errorf("test setup: %s is not removed for maxsize even without the fault:\n%s", filepath.Join(part, old), out)
			}
		})
	}
}

// r2l26HistoryEx returns what dilookup says of the history record of msgid: valid, expired
// or rejected.
func r2l26HistoryEx(tb testing.TB, s *Site, msgid string) string {
	tb.Helper()
	out := s.Run(tb, "dilookup", msgid)
	i := strings.Index(out, "EX=(")
	if i < 0 {
		tb.Fatalf("dilookup %s: no EX=(...) in %q", msgid, out)
	}
	ex, _, _ := strings.Cut(out[i+len("EX=("):], ")")
	return ex
}

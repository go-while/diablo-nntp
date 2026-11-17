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

// sleepGrowSource replaces dexpire's sleep(10) between two passes with one that returns at
// once and, on its first call only, appends SLEEPGROW_BYTES bytes to the file SLEEPGROW_FILE:
// what a running diablo writes into the spool's newest directory while dexpire sleeps. Other
// sleeps (lock waits) are the real ones. After that first sleep(10), opendir() of the
// directory whose last path component is SLEEPFAIL_NAME (if set) fails with EIO, and the path
// is appended to SLEEPFAIL_MARK: a read error in pass 2 only. On NetBSD <dirent.h> renames
// opendir() to __opendir30 (see dirFailSource).
const sleepGrowSource = `#define _GNU_SOURCE
#define __EXTENSIONS__
#if defined(__NetBSD__)
#define REAL_OPENDIR "__opendir30"
#else
#define REAL_OPENDIR "opendir"
#endif
#include <dirent.h>
#include <dlfcn.h>
#include <errno.h>
#include <fcntl.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>
static int passes;	/* sleep(10) calls so far */

unsigned int
sleep(unsigned int secs)
{
    static unsigned int (*real)(unsigned int);
    const char *file = getenv("SLEEPGROW_FILE");
    const char *bytes = getenv("SLEEPGROW_BYTES");
    char buf[4096];
    long n;
    int fd;

    if (secs != 10) {
	if (real == NULL)
	    real = (unsigned int (*)(unsigned int))dlsym(RTLD_NEXT, "sleep");
	return real(secs);
    }
    if (passes++ > 0 || file == NULL || bytes == NULL)
	return 0;
    if ((fd = open(file, O_WRONLY | O_CREAT | O_APPEND, 0644)) < 0)
	return 0;
    memset(buf, 'x', sizeof(buf));
    for (n = atol(bytes); n > 0; n -= (long)sizeof(buf)) {
	if (write(fd, buf, n < (long)sizeof(buf) ? (size_t)n : sizeof(buf)) < 0)
	    break;
    }
    close(fd);
    return 0;
}

DIR *
opendir(const char *path)
{
    static DIR *(*real)(const char *);
    const char *name = getenv("SLEEPFAIL_NAME");
    const char *mark = getenv("SLEEPFAIL_MARK");
    size_t l = strlen(path);
    size_t nl;
    FILE *f;

    if (real == NULL)
	real = (DIR *(*)(const char *))dlsym(RTLD_NEXT, REAL_OPENDIR);
    if (passes == 0 || name == NULL)
	return real(path);
    while (l > 1 && path[l - 1] == '/')
	--l;
    nl = strlen(name);
    if (l < nl || strncmp(path + l - nl, name, nl) != 0 ||
				(l > nl && path[l - nl - 1] != '/'))
	return real(path);
    if (mark != NULL && (f = fopen(mark, "a")) != NULL) {
	fprintf(f, "%s\n", path);
	fclose(f);
    }
    errno = EIO;
    return NULL;
}
`

// TestR2DexpireSecondPassHistory (deep review r2 L25, D-267): dexpire runs another pass over
// the spools after one that removed something, and each pass's history update keeps the
// records of the directories that pass listed. The list (NodeAry) was never cleared, so a
// directory listed in pass 1 and removed in pass 2 kept its records unexpired.
//
// Spool 00 with maxsize 1m holds a 1000 KB directory an hour old, the directory of two stored
// articles, and a newer one of 100 KB. Pass 1 removes the old one, which is enough. Between
// the passes (a preloaded sleep(10) that returns at once) 1 MB is written into the newest
// directory, as a running diablo does, so pass 2 removes the articles' directory. Its records
// must now be marked expired (dilookup: EX=(expired)). STAT cannot tell: the feeder answers
// 430 for a record whose file is gone, expired or not.
//
// Case pass2_read_error: in pass 2 the spool directory cannot be read (opendir() fails with
// EIO). Pass 2 lists nothing of spool 00 and removes nothing; the articles are still on disk,
// so their records must stay valid. The stale list used to cover this by accident; with the
// list cleared per pass, the per-directory mark of D-268 (L26) must.
func TestR2DexpireSecondPassHistory(t *testing.T) {
	so := preloadShim(t, "sleepgrow", sleepGrowSource)
	for _, tc := range []struct {
		name string
		fail string // SLEEPFAIL_NAME: directory that cannot be read in pass 2
	}{
		{"removed_in_pass2", ""},
		{"pass2_read_error", "news"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := NewSite(t, SiteOptions{SpoolCtl: "spool 00\n    maxsize 1m\nend\nmetaspool trad\n    spool 00\nend\nexpire * trad\n"})
			ids := s.Inject(t, "e2e.test", 2)
			top := filepath.Join(s.Dir, "spool", "news")
			// one directory, or two when the articles straddle a 10-minute boundary
			mid, _ := filepath.Glob(filepath.Join(top, "D.*"))
			if len(mid) == 0 || len(mid) > 2 {
				t.Fatalf("spool directories: %q, want one or two", mid)
			}
			now := time.Now().Unix() / 60
			old := filepath.Join(top, fmt.Sprintf("D.%08x", now-60))
			newest := filepath.Join(top, fmt.Sprintf("D.%08x", now+1))
			for _, d := range []struct {
				dir  string
				size int
			}{{old, 1000 * 1024}, {newest, 100 * 1024}} {
				if err := os.Mkdir(d.dir, 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(d.dir, "B.0001"), make([]byte, d.size), 0o644); err != nil {
					t.Fatal(err)
				}
			}

			grow := filepath.Join(newest, "B.0002")
			mark := filepath.Join(t.TempDir(), "sleepfail")
			base := s.Env
			s.Env = append(slices.Clone(base), lockFaultEnv(so), "SLEEPGROW_FILE="+grow, "SLEEPGROW_BYTES=1048576")
			if tc.fail != "" {
				s.Env = append(s.Env, "SLEEPFAIL_NAME="+tc.fail, "SLEEPFAIL_MARK="+mark)
			}
			out, err := s.TryRun("dexpire", "-a", "-h1", "-v")
			s.Env = base
			if err != nil {
				t.Fatalf("dexpire -a -h1: %v\n%s", err, out)
			}
			if fi, err := os.Stat(grow); err != nil || fi.Size() != 1048576 {
				t.Fatalf("test setup: the newest directory did not grow between the passes (%v):\n%s", err, out)
			}
			if _, err := os.Stat(old); err == nil {
				t.Fatalf("test setup: the old directory was not removed:\n%s", out)
			}
			if !strings.Contains(out, "Spool scan pass 2") {
				t.Fatalf("test setup: no second pass:\n%s", out)
			}
			if _, err := os.Stat(newest); err != nil {
				t.Errorf("the newest directory was removed: %v", err)
			}

			want := "expired"
			if tc.fail != "" {
				if b, err := os.ReadFile(mark); err != nil || len(b) == 0 {
					t.Fatalf("test setup: the read error in pass 2 was never hit (%v):\n%s", err, out)
				}
				for _, d := range mid {
					if _, err := os.Stat(d); err != nil {
						t.Fatalf("test setup: %s was removed although pass 2 could not read the spool: %v\n%s", d, err, out)
					}
				}
				want = "valid"
			} else {
				for _, d := range mid {
					if _, err := os.Stat(d); err == nil {
						t.Fatalf("test setup: the second pass did not remove %s:\n%s", d, out)
					}
				}
			}
			for _, id := range ids {
				if ex := r2l25HistoryEx(t, s, id); ex != want {
					t.Errorf("history record of %s: EX=(%s), want %s", id, ex, want)
				}
			}
			if t.Failed() {
				t.Logf("dexpire -a -h1 -v:\n%s", out)
			}
		})
	}
}

// r2l25HistoryEx returns what dilookup says of the history record of msgid: valid, expired
// or rejected.
func r2l25HistoryEx(tb testing.TB, s *Site, msgid string) string {
	tb.Helper()
	out := s.Run(tb, "dilookup", msgid)
	i := strings.Index(out, "EX=(")
	if i < 0 {
		tb.Fatalf("dilookup %s: no EX=(...) in %q", msgid, out)
	}
	ex, _, _ := strings.Cut(out[i+len("EX=("):], ")")
	return ex
}

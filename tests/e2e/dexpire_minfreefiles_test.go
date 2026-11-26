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

// statfsZeroSource answers every statfs() (statvfs() where dexpire uses it: NetBSD, illumos)
// as a file system without a fixed number of inodes does (btrfs): f_files and f_ffree 0. The
// free space is made up as well: 1 TiB of 4 KB blocks, all free, or STATFSZERO_BAVAIL blocks
// free. The path asked about is appended to the file STATFSZERO_MARK. The real call is never
// made, so the shim needs no symbol name of it (NetBSD renames statvfs() in <sys/statvfs.h>;
// the definition here gets that name from the header).
const statfsZeroSource = `#define _GNU_SOURCE
#define __EXTENSIONS__
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#if defined(__linux__)
#include <sys/vfs.h>
#elif defined(__NetBSD__) || defined(__sun)
#include <sys/statvfs.h>
#define FAKE_STATVFS 1
#else
#include <sys/param.h>
#include <sys/mount.h>
#endif

#define FAKEBLOCKS (1ULL << 28)		/* 1 TiB of 4 KB blocks */

static void
mark(const char *path)
{
    const char *m = getenv("STATFSZERO_MARK");
    FILE *f;

    if (m != NULL && (f = fopen(m, "a")) != NULL) {
	fprintf(f, "%s\n", path);
	fclose(f);
    }
}

static unsigned long long
avail(void)
{
    const char *a = getenv("STATFSZERO_BAVAIL");

    return a != NULL ? strtoull(a, NULL, 10) : FAKEBLOCKS;
}

#define FILL(st) do {					\
	memset((st), 0, sizeof(*(st)));			\
	(st)->f_bsize = 4096;				\
	(st)->f_blocks = FAKEBLOCKS;			\
	(st)->f_bfree = avail();			\
	(st)->f_bavail = avail();			\
	(st)->f_files = 0;				\
	(st)->f_ffree = 0;				\
    } while (0)

#ifdef FAKE_STATVFS
int
statvfs(const char *path, struct statvfs *st)
{
    mark(path);
    FILL(st);
    st->f_frsize = 4096;
    return 0;
}
#else
int
statfs(const char *path, struct statfs *st)
{
    mark(path);
    FILL(st);
#ifdef __linux__
    st->f_frsize = 4096;
#endif
    return 0;
}

#ifdef __GLIBC__	/* the name statfs() has with _FILE_OFFSET_BITS=64 */
int
statfs64(const char *path, struct statfs64 *st)
{
    mark(path);
    FILL(st);
    st->f_frsize = 4096;
    return 0;
}
#endif
#endif
`

// TestDexpireMinFreeFilesNoInodeCount (D-273): btrfs has no fixed number of inodes and reports
// f_files and f_ffree as 0. dexpire read the 0 free files as "no free inodes", so with
// minfreefiles set it removed every spool directory but the newest at each run. Now a file
// system with f_files 0 leaves the free files unknown: minfreefiles is not applied there (a
// message says so), while the other limits still are.
//
// Spool 00 has directories 3 h, 50, 40 and 30 minutes old and a newest one 5 minutes old, and
// a preloaded library answers statfs() as btrfs does (plenty of free space).
//   - inodes_unknown: minfreefiles 1000 and keeptime 3600: only the 3 h old directory goes
//     (keeptime), the others stay. Fails before the fix: all but the newest are removed.
//   - space_low: minfree 1m as well and no free blocks: the free-space check still applies,
//     so all but the newest go (passes before and after the fix; the message is not checked).
func TestDexpireMinFreeFilesNoInodeCount(t *testing.T) {
	so := preloadShim(t, "statfszero", statfsZeroSource)
	for _, tc := range []struct {
		name    string
		limits  string
		bavail  string
		removed int  // the oldest n directories are removed
		message bool // the "no inode count" message must be there
	}{
		{"inodes_unknown", "    minfreefiles 1000\n    keeptime 3600\n", "", 1, true},
		{"space_low", "    minfree 1m\n    minfreefiles 1000\n    keeptime 3600\n", "0", 4, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			now := time.Now().Unix() / 60
			var dirs []string // oldest first; the last one is the newest
			s := NewSite(t, SiteOptions{
				NoStart:  true,
				SpoolCtl: "spool 00\n" + tc.limits + "end\nmetaspool m\n    spool 00\nend\nexpire * m\n",
				Setup: func(tb testing.TB, s *Site) {
					top := filepath.Join(s.Dir, "spool", "news")
					for _, age := range []int64{180, 50, 40, 30, 5} {
						d := filepath.Join(top, fmt.Sprintf("D.%08x", now-age))
						if err := os.MkdirAll(d, 0o755); err != nil {
							tb.Fatal(err)
						}
						if err := os.WriteFile(filepath.Join(d, "B.0001"), []byte("article\n"), 0o644); err != nil {
							tb.Fatal(err)
						}
						dirs = append(dirs, d)
					}
				},
			})
			mark := filepath.Join(t.TempDir(), "statfs")
			env := []string{lockFaultEnv(so), "STATFSZERO_MARK=" + mark}
			if tc.bavail != "" {
				env = append(env, "STATFSZERO_BAVAIL="+tc.bavail)
			}
			base := s.Env
			s.Env = append(slices.Clone(base), env...)
			out, err := s.TryRun("dexpire", "-a", "-h0", "-v")
			s.Env = base
			if b, err := os.ReadFile(mark); err != nil || len(b) == 0 {
				t.Fatalf("the shim was never asked (%v):\n%s", err, out)
			}
			if err != nil {
				t.Errorf("dexpire -a -h0: %v", err)
			}
			for i, d := range dirs {
				_, serr := os.Stat(d)
				switch {
				case i < tc.removed && serr == nil:
					t.Errorf("%s (%d of %d, oldest first) is still there, want it removed", filepath.Base(d), i+1, len(dirs))
				case i >= tc.removed && serr != nil:
					t.Errorf("%s (%d of %d, oldest first) is gone (%v), want it kept", filepath.Base(d), i+1, len(dirs), serr)
				}
			}
			const msg = "reports no inode count: not expiring for minfreefiles there"
			if tc.message && !strings.Contains(out, msg) {
				t.Errorf("no message %q in the output", msg)
			}
			if n, passes := strings.Count(out, msg), strings.Count(out, "Spool scan pass"); n > passes && passes > 0 {
				t.Errorf("message given %d times in %d passes, want at most once per pass", n, passes)
			}
			if t.Failed() {
				t.Logf("dexpire -a -h0 -v: err=%v\n%s", err, out)
			}
		})
	}
}

package e2e

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"
)

// TestDexpirecacheDefaultDir: without -d, dexpirecache works on the configured cache
// directory (path_cache) and reads diablo.config given with -C. Before, -d had no default
// (chdir(NULL): "Unable to change to `(null)'") and -C was an unknown option.
func TestDexpirecacheDefaultDir(t *testing.T) {
	s := NewSite(t, SiteOptions{NoStart: true})
	if err := os.MkdirAll(filepath.Join(s.Dir, "spool/cache"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"-v", "-f", "1"},
		{"-v", "-f", "1", "-C", filepath.Join(s.Dir, "diablo.config")},
	} {
		out, err := s.TryRun("dexpirecache", args...)
		if err != nil || !strings.Contains(out, "No need to expire cache") {
			t.Errorf("dexpirecache %s: %v\n%s", strings.Join(args, " "), err, out)
		}
	}
}

// TestDexpirecacheNoInodeCount (D-299): btrfs has no fixed number of inodes and reports
// f_files and f_ffree as 0. dexpirecache -i read the 0 free inodes as "none free", so at every
// run it removed the oldest empty cache files (articles kept as uncacheable) and symbolic
// links, as if the inodes had run out. Now a file system with f_files 0 leaves the free inodes
// unknown: -i is not applied there (a message says so), while -f still is.
//
// The cache has two cache files with content and three empty files and a symbolic link (what
// -i removes), all an hour old, and a preloaded library answers statfs() as btrfs does.
//   - inodes_unknown: -i 1000, plenty of free space: nothing is removed. Fails before the fix:
//     at least one empty file or link is removed.
//   - space_low: -f 1000 -i 1000, no free blocks: the two cache files go (-f), the empty
//     files and the link stay. Fails before the fix in the same way.
//   - inodes_known: -i 1000 and the library reports 1 Mi inodes, 10 free: -i still applies
//     where the count is known, so at least one empty file or link goes (passes before and
//     after the fix).
func TestDexpirecacheNoInodeCount(t *testing.T) {
	so := preloadShim(t, "statfszero", statfsZeroSource)
	const msg = "reports no inode count: not expiring for -i there"
	for _, tc := range []struct {
		name        string
		args        []string
		env         []string
		markersGone bool // at least one empty file or link removed (else all kept)
		filesGone   bool // both cache files with content removed (else both kept)
		message     bool // the "no inode count" message must be there (else must not)
	}{
		{"inodes_unknown", []string{"-i", "1000"}, nil, false, false, true},
		{"space_low", []string{"-f", "1000", "-i", "1000"}, []string{"STATFSZERO_BAVAIL=0"}, false, true, true},
		{"inodes_known", []string{"-i", "1000"}, []string{"STATFSZERO_FFREE=10"}, true, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := NewSite(t, SiteOptions{NoStart: true})
			cache := filepath.Join(s.Dir, "spool", "cache")
			dir := filepath.Join(cache, "00")
			if err := os.MkdirAll(dir, 0o755); err != nil {
				t.Fatal(err)
			}
			old := time.Now().Add(-time.Hour)
			var files, markers []string
			for _, n := range []string{"a1", "a2"} {
				p := filepath.Join(dir, n)
				f, err := os.Create(p)
				if err != nil {
					t.Fatal(err)
				}
				_, err = f.Write([]byte(strings.Repeat("article line\r\n", 600)))
				if err == nil {
					err = f.Sync()
				}
				if cerr := f.Close(); err == nil {
					err = cerr
				}
				if err != nil {
					t.Fatal(err)
				}
				if err := os.Chtimes(p, old, old); err != nil {
					t.Fatal(err)
				}
				files = append(files, p)
			}
			for _, n := range []string{"e1", "e2", "e3"} {
				p := filepath.Join(dir, n)
				if err := os.WriteFile(p, nil, 0o644); err != nil {
					t.Fatal(err)
				}
				if err := os.Chtimes(p, old, old); err != nil {
					t.Fatal(err)
				}
				markers = append(markers, p)
			}
			l := filepath.Join(dir, "l1")
			if err := os.Symlink("e1", l); err != nil {
				t.Fatal(err)
			}
			markers = append(markers, l)
			if tc.filesGone {
				// dexpirecache counts a regular file with no blocks as an empty one
				for _, p := range files {
					if st, err := os.Lstat(p); err != nil || st.Sys().(*syscall.Stat_t).Blocks == 0 {
						t.Skipf("%s shows no blocks on this file system (%v)", p, err)
					}
				}
			}

			mark := filepath.Join(t.TempDir(), "statfs")
			base := s.Env
			s.Env = append(slices.Clone(base), append([]string{lockFaultEnv(so), "STATFSZERO_MARK=" + mark}, tc.env...)...)
			args := append([]string{"-d", cache, "-v"}, tc.args...)
			out, err := s.TryRun("dexpirecache", args...)
			s.Env = base
			if b, rerr := os.ReadFile(mark); rerr != nil || len(b) == 0 {
				t.Fatalf("the shim was never asked (%v):\n%s", rerr, out)
			}
			if err != nil {
				t.Errorf("dexpirecache %s: %v", strings.Join(args, " "), err)
			}
			gone := func(p string) bool { _, err := os.Lstat(p); return err != nil }
			n := 0
			for _, p := range markers {
				if gone(p) {
					n++
				}
			}
			switch {
			case tc.markersGone && n == 0:
				t.Errorf("no empty file or link removed, want the inode limit applied")
			case !tc.markersGone && n > 0:
				t.Errorf("%d of %d empty files and links removed, want all kept", n, len(markers))
			}
			for _, p := range files {
				if g := gone(p); g != tc.filesGone {
					t.Errorf("cache file %s removed: %v, want %v", filepath.Base(p), g, tc.filesGone)
				}
			}
			if c := strings.Count(out, msg); tc.message && c != 1 {
				t.Errorf("message %q given %d times, want once", msg, c)
			} else if !tc.message && c != 0 {
				t.Errorf("message %q given, want none", msg)
			}
			if t.Failed() {
				t.Logf("dexpirecache %s: err=%v\n%s", strings.Join(args, " "), err, out)
			}
		})
	}
}

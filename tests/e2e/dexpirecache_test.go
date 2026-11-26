package e2e

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
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

// TestDexpirecacheInodeOrder (D-302): dexpirecache -i N should remove the N oldest empty
// cache files and symbolic links (by ctime, oldest first), ties in the order of their names.
// addSLink() kept its sorted list's end at the first entry met in directory order and left out
// every newer entry, and an entry with the same ctime as the list's end was never linked in,
// while a full list kept N+1 entries. So -i N removed only the first entry met and the older
// ones, at most N+1.
//
// In four, one and all the cache has seven empty files and links made in three seconds:
// 00/a1 (empty file) and 01/a2 (link) in the first, 00/b1, 00/b2 and 01/b3 (empty files) in
// the second, 00/c1 (empty file) and 01/c2 (link) in the third. Two cache files with content
// are an hour old, and a preloaded library reports 1 Mi inodes, none free, so that all N
// inodes are needed.
//   - four: -i 4 removes ./00/a1, ./01/a2, ./00/b1, ./00/b2, in that order. Fails before the
//     fix whatever the directory order: 1, 3 or 5 entries are removed (the first met is in
//     the first, second or third second).
//   - one: -i 1 removes ./00/a1 only.
//   - all: -i 10 removes all seven, oldest first.
//   - same: five made in one second instead (00/s1, 00/s2, 00/s5 empty files, 01/s3 a link,
//     01/s4 an empty file): -i 3 removes ./00/s1, ./00/s2, ./00/s5, in the order of the names.
//     Fails before the fix whatever the directory order: one entry is removed (the first met;
//     the others have its ctime and were never linked in).
//
// The two cache files with content stay in each case (-i does not remove them).
func TestDexpirecacheInodeOrder(t *testing.T) {
	so := preloadShim(t, "statfszero", statfsZeroSource)
	threeSeconds := [][]string{{"00/a1", "01/a2"}, {"00/b1", "00/b2", "01/b3"}, {"00/c1", "01/c2"}}
	links := map[string]bool{"01/a2": true, "01/c2": true, "01/s3": true}
	for _, tc := range []struct {
		name   string
		groups [][]string // made one second after another, in each the order of the names
		n      int
	}{
		{"four", threeSeconds, 4},
		{"one", threeSeconds, 1},
		{"all", threeSeconds, 10},
		{"same", [][]string{{"00/s1", "00/s2", "00/s5", "01/s3", "01/s4"}}, 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			groups := tc.groups
			var order []string // oldest first
			for _, g := range groups {
				order = append(order, g...)
			}
			s := NewSite(t, SiteOptions{NoStart: true})
			cache := filepath.Join(s.Dir, "spool", "cache")
			for _, d := range []string{"00", "01"} {
				if err := os.MkdirAll(filepath.Join(cache, d), 0o755); err != nil {
					t.Fatal(err)
				}
			}
			old := time.Now().Add(-time.Hour)
			var files []string
			for _, n := range []string{"00/f1", "01/f2"} {
				p := filepath.Join(cache, n)
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
				// dexpirecache counts a regular file with no blocks as an empty one
				if st, err := os.Lstat(p); err != nil || st.Sys().(*syscall.Stat_t).Blocks == 0 {
					t.Skipf("%s shows no blocks on this file system (%v)", p, err)
				}
				files = append(files, p)
			}
			// Each group is made just after a whole second; a new file's mtime is its
			// ctime, so the mtimes show that each group shares one ctime second.
			var secs []int64
			for _, g := range groups {
				now := time.Now()
				time.Sleep(now.Truncate(time.Second).Add(time.Second + 100*time.Millisecond).Sub(now))
				for _, n := range g {
					p := filepath.Join(cache, n)
					var err error
					if links[n] {
						err = os.Symlink("f1", p)
					} else {
						err = os.WriteFile(p, nil, 0o644)
					}
					if err != nil {
						t.Fatal(err)
					}
				}
				for _, n := range g {
					st, err := os.Lstat(filepath.Join(cache, n))
					if err != nil {
						t.Fatal(err)
					}
					secs = append(secs, st.ModTime().Unix())
				}
			}
			for i, k := 0, 0; i < len(groups); i++ {
				for j := range groups[i] {
					if secs[k+j] != secs[k] || (k > 0 && secs[k] <= secs[k-1]) {
						t.Fatalf("setup: the groups are not in one second each: %v", secs)
					}
				}
				k += len(groups[i])
			}

			mark := filepath.Join(t.TempDir(), "statfs")
			base := s.Env
			s.Env = append(slices.Clone(base), lockFaultEnv(so), "STATFSZERO_MARK="+mark, "STATFSZERO_FFREE=0")
			args := []string{"-d", cache, "-v", "-i", strconv.Itoa(tc.n)}
			out, err := s.TryRun("dexpirecache", args...)
			s.Env = base
			if b, rerr := os.ReadFile(mark); rerr != nil || len(b) == 0 {
				t.Fatalf("the shim was never asked (%v):\n%s", rerr, out)
			}
			if err != nil {
				t.Errorf("dexpirecache %s: %v", strings.Join(args, " "), err)
			}

			want := order[:min(tc.n, len(order))]
			var removed []string
			for _, l := range strings.Split(out, "\n") {
				if r, ok := strings.CutPrefix(l, "Removing ./"); ok {
					removed = append(removed, r)
				}
			}
			if !slices.Equal(removed, want) {
				t.Errorf("removed %q, want %q", removed, want)
			}
			gone := func(p string) bool { _, err := os.Lstat(p); return err != nil }
			for _, n := range order {
				if g, w := gone(filepath.Join(cache, n)), slices.Contains(want, n); g != w {
					t.Errorf("%s removed: %v, want %v", n, g, w)
				}
			}
			for _, p := range files {
				if gone(p) {
					t.Errorf("cache file %s removed, want kept", filepath.Base(p))
				}
			}
			if t.Failed() {
				t.Logf("dexpirecache %s: err=%v\n%s", strings.Join(args, " "), err, out)
			}
		})
	}
}

// TestDexpirecacheReport (D-303): the statistics at the end of a run. The empty cache files and
// symbolic links that -i removes (removeLinks()), and those addSLink() removes because their
// ctime is more than two days old, were not counted anywhere: a run that removed only those
// said "0 files have been cleaned". And a run that removed no cache file with content printed
// "Oldest removed file was created 0 days 0 hrs 0 min 0 sec ago" and "Newest removed file was
// created 20783 days 0 hrs 13 min 28 sec ago" (on 2026-11-26: newestremoved starts at 0, the
// epoch). Now a line "* N empty files and links removed" counts each entry removed once, and
// the two age lines are printed only when a cache file with content was removed.
//
// The cache has two empty files and a symbolic link (made now), and in all but none two cache
// files with content an hour old. A preloaded library answers statfs() (no free blocks with
// -f, no free inodes of 1 Mi with -i) and in old_* moves time() three days ahead
// (timeShiftSource). Every case fails before the fix, as there was no "empty files and links
// removed" line; inodes, none and old_inodes also because both age lines were printed.
//   - inodes: -i 3: the three entries go; 0 files cleaned, 3 empty files and links removed, no
//     age lines.
//   - space: -f 1000: the two cache files go, the entries stay; 2 files cleaned, 0 empty files
//     and links removed, both age lines (less than a day).
//   - none: -f 1000 with no cache file with content: nothing goes; 0 and 0, no age lines.
//   - old_space: -f 1000, three days on: the cache files go (-f), the entries stay (the
//     two-day rule in addSLink() is part of -i now; before, it removed them with -f alone too);
//     2 cleaned, 0 empty files and links removed, both age lines (3 days). Fails before the
//     change as the entries were removed and counted (3).
//   - old_inodes: -i 3, three days on: the two-day rule removes the entries before
//     removeLinks() does; they are counted once: 3.
func TestDexpirecacheReport(t *testing.T) {
	so := preloadShim(t, "statfstime", statfsZeroSource+withMark(timeShiftSource))
	for _, tc := range []struct {
		name        string
		args        []string
		env         []string
		shifted     bool // time() three days ahead
		content     bool // two cache files with content, an hour old
		contentGone bool
		entriesGone bool
		cleaned     int    // "files have been cleaned"
		links       int    // "empty files and links removed"
		ages        string // the age lines must begin so ("" = no age lines)
	}{
		{"inodes", []string{"-i", "3"}, []string{"STATFSZERO_FFREE=0"}, false, true, false, true, 0, 3, ""},
		{"space", []string{"-f", "1000"}, []string{"STATFSZERO_BAVAIL=0"}, false, true, true, false, 2, 0, "0 days "},
		{"none", []string{"-f", "1000"}, []string{"STATFSZERO_BAVAIL=0"}, false, false, false, false, 0, 0, ""},
		{"old_space", []string{"-f", "1000"}, []string{"STATFSZERO_BAVAIL=0"}, true, true, true, false, 2, 0, "3 days "},
		{"old_inodes", []string{"-i", "3"}, []string{"STATFSZERO_FFREE=0"}, true, true, false, true, 0, 3, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := NewSite(t, SiteOptions{NoStart: true})
			cache := filepath.Join(s.Dir, "spool", "cache")
			for _, d := range []string{"00", "01"} {
				if err := os.MkdirAll(filepath.Join(cache, d), 0o755); err != nil {
					t.Fatal(err)
				}
			}
			var files []string
			if tc.content {
				old := time.Now().Add(-time.Hour)
				for _, n := range []string{"00/f1", "01/f2"} {
					p := filepath.Join(cache, n)
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
					// dexpirecache counts a regular file with no blocks as an empty one
					if st, err := os.Lstat(p); err != nil || st.Sys().(*syscall.Stat_t).Blocks == 0 {
						t.Skipf("%s shows no blocks on this file system (%v)", p, err)
					}
					files = append(files, p)
				}
			}
			var entries []string
			for _, n := range []string{"00/e1", "00/e2", "01/l1"} {
				p := filepath.Join(cache, n)
				var err error
				if n == "01/l1" {
					err = os.Symlink("f2", p)
				} else {
					err = os.WriteFile(p, nil, 0o644)
				}
				if err != nil {
					t.Fatal(err)
				}
				entries = append(entries, p)
			}

			tmp := t.TempDir()
			mark := filepath.Join(tmp, "statfs")
			env := append([]string{lockFaultEnv(so), "STATFSZERO_MARK=" + mark}, tc.env...)
			if tc.shifted {
				shift := filepath.Join(tmp, "shift")
				if err := os.WriteFile(shift, []byte("259200\n"), 0o644); err != nil {
					t.Fatal(err)
				}
				env = append(env, "TIMESHIFT_FILE="+shift)
			}
			base := s.Env
			s.Env = append(slices.Clone(base), env...)
			args := append([]string{"-d", cache, "-v"}, tc.args...)
			out, err := s.TryRun("dexpirecache", args...)
			s.Env = base
			if b, rerr := os.ReadFile(mark); rerr != nil || len(b) == 0 {
				t.Fatalf("the shim was never asked (%v):\n%s", rerr, out)
			}
			if err != nil {
				t.Errorf("dexpirecache %s: %v", strings.Join(args, " "), err)
			}
			if tc.shifted && !strings.Contains(out, "Oldest file found was created 3 days ") {
				t.Fatalf("time() was not moved three days on:\n%s", out)
			}

			gone := func(p string) bool { _, err := os.Lstat(p); return err != nil }
			for _, p := range files {
				if g := gone(p); g != tc.contentGone {
					t.Errorf("cache file %s removed: %v, want %v", filepath.Base(p), g, tc.contentGone)
				}
			}
			for _, p := range entries {
				if g := gone(p); g != tc.entriesGone {
					t.Errorf("entry %s removed: %v, want %v", filepath.Base(p), g, tc.entriesGone)
				}
			}
			for _, want := range []string{
				fmt.Sprintf("* %7d files have been cleaned\n", tc.cleaned),
				fmt.Sprintf("* %7d empty files and links removed\n", tc.links),
			} {
				if !strings.Contains(out, want) {
					t.Errorf("no line %q", strings.TrimSuffix(want, "\n"))
				}
			}
			for _, l := range []string{"Oldest removed file was created ", "Newest removed file was created "} {
				var got []string
				for _, line := range strings.Split(out, "\n") {
					if strings.HasPrefix(line, l) {
						got = append(got, line)
					}
				}
				switch {
				case tc.ages == "" && len(got) > 0:
					t.Errorf("age line printed with no cache file removed: %q", got)
				case tc.ages != "" && (len(got) != 1 || !strings.HasPrefix(got[0], l+tc.ages)):
					t.Errorf("age lines %q, want one beginning %q", got, l+tc.ages)
				}
			}
			if t.Failed() {
				t.Logf("dexpirecache %s: err=%v\n%s", strings.Join(args, " "), err, out)
			}
		})
	}
}

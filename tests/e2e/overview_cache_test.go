package e2e

// readerovcache: the number of groups whose overview a dreaderd fork keeps open.  A cached
// group keeps its over.* index open with a shared 4-byte fcntl lock at offset 4 (the lock
// dexpireover checks before it resizes); a closed one has no lock.  The tests feed one
// article per group to the feed-only fork and then ask, from the test, whether each
// group's index is still locked.  The feed-only fork flushes its cache only every 300 s, so
// nothing else closes a group during the test.
//
//	KeepsGroupsOpen  readerovcache 16, 12 groups: all 12 stay open.  On main the cache was
//	                 a fixed 8 groups, and readerovcache was an unknown option.
//	LeastRecent      readerovcache 4: groups 1-4, then 1 again, then 5.  The group used
//	                 least recently (2) is closed, 1 stays open.  On main a full cache
//	                 dropped half of its groups in hash-bucket order.
//	Lowered          readerovcache 65536 does not fit the descriptor limit: dreaderd logs
//	                 the lower number it uses, and the feed still works.

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	testkit "github.com/go-while/nntp-testkit"
)

// overIndexPath finds the over.* index of group under the site's spool/group: the file
// whose OverHead names the group (version, byteOrder, headSize, maxArts, then gName[256]).
func overIndexPath(tb testing.TB, s *Site, group string) string {
	tb.Helper()
	want := append([]byte(group), 0)
	found := ""
	err := filepath.WalkDir(filepath.Join(s.Dir, "spool", "group"), func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || found != "" {
			return err
		}
		f, err := os.Open(p)
		if err != nil {
			return nil
		}
		defer f.Close()
		head := make([]byte, 16+256)
		if n, _ := f.Read(head); n == len(head) && bytes.HasPrefix(head[16:], want) {
			found = p
		}
		return nil
	})
	if err != nil {
		tb.Fatal(err)
	}
	if found == "" {
		tb.Fatalf("no over.* index for %s under spool/group", group)
	}
	return found
}

// overIndexHeld reports whether another process holds a lock on the over.* index of group
// at offset 4 (dreaderd's shared lock while the group is cached).  F_GETLK only asks.
func overIndexHeld(tb testing.TB, s *Site, group string) bool {
	tb.Helper()
	f, err := os.OpenFile(overIndexPath(tb, s, group), os.O_RDWR, 0)
	if err != nil {
		tb.Fatal(err)
	}
	defer f.Close()
	lk := syscall.Flock_t{Type: syscall.F_WRLCK, Whence: 0, Start: 4, Len: 4}
	if err := syscall.FcntlFlock(f.Fd(), syscall.F_GETLK, &lk); err != nil {
		tb.Fatalf("F_GETLK on the index of %s: %v", group, err)
	}
	return lk.Type != syscall.F_UNLCK
}

// ovCacheSite starts a reader site with groups e2e.ovc.01..n and the given readerovcache
// ("" leaves the option out), and returns it with a function feeding one article to a group.
func ovCacheSite(t *testing.T, n int, ovcache string) (*Site, []string, func(group string)) {
	t.Helper()
	st, spool := spoolStore(t)
	var groups []string
	for i := 1; i <= n; i++ {
		g := fmt.Sprintf("e2e.ovc.%02d", i)
		groups = append(groups, g)
		st.AddGroup(g, "overview cache test group")
	}
	conf := map[string]string{}
	if ovcache != "" {
		conf["readerovcache"] = ovcache
	}
	s := NewReaderSite(t, ReaderOptions{Spool: spool, Groups: groups, Config: conf})
	return s, groups, func(group string) {
		t.Helper()
		feedReader(t, s, st, group, newArticle(msgID(t), group, "ovcache "+group, "body"))
	}
}

func TestReaderOvCacheKeepsGroupsOpen(t *testing.T) {
	s, groups, feed := ovCacheSite(t, 12, "16")
	for _, g := range groups {
		feed(g)
	}
	var closed []string
	for _, g := range groups {
		if !overIndexHeld(t, s, g) {
			closed = append(closed, g)
		}
	}
	if len(closed) != 0 {
		t.Errorf("readerovcache 16, %d groups fed: %d closed again (%s), want all kept open",
			len(groups), len(closed), strings.Join(closed, " "))
	}
}

func TestReaderOvCacheLeastRecent(t *testing.T) {
	s, g, feed := ovCacheSite(t, 5, "4")
	for _, grp := range g[:4] {
		feed(grp)
	}
	feed(g[0]) // 1 is now the most recently used, 2 the least
	feed(g[4]) // the cache is full: one group is closed
	want := map[string]bool{g[0]: true, g[1]: false, g[2]: true, g[3]: true, g[4]: true}
	for _, grp := range g {
		if got := overIndexHeld(t, s, grp); got != want[grp] {
			t.Errorf("%s open %v, want %v (readerovcache 4: fed 1 2 3 4 1 5, so 2 is closed)",
				grp, got, want[grp])
		}
	}
}

func TestReaderOvCacheLowered(t *testing.T) {
	s, groups, feed := ovCacheSite(t, 2, "65536")
	var line string
	testkit.WaitFor(t, 10*time.Second, "the lowered readerovcache in general.log", func() bool {
		b, _ := os.ReadFile(filepath.Join(s.Dir, "log", "general.log"))
		for _, l := range strings.Split(string(b), "\n") {
			if strings.Contains(l, "readerovcache 65536 lowered to ") {
				line = l
				return true
			}
		}
		return false
	})
	var n int
	if _, err := fmt.Sscanf(line[strings.Index(line, "lowered to ")+len("lowered to "):], "%d", &n); err != nil || n < 8 || n >= 65536 {
		t.Errorf("log line %q: want a number from 8 to 65535", line)
	}
	for _, g := range groups {
		feed(g)
		if !overIndexHeld(t, s, g) {
			t.Errorf("%s: not open after the feed", g)
		}
	}
}

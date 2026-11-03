package e2e

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

// groupListAccess is DefaultReaderAccess with two group lists, "all" (the first, everything)
// and "testonly" (e2e.test), and the readerdef line given.
func groupListAccess(line string) string {
	return accessWith(line) + `groupdef all
    group *
end
groupdef testonly
    group e2e.test
end
`
}

// activeNames returns the sorted, distinct group names of LIST ACTIVE lines. Distinct, as
// on OpenBSD a stale map can list a group more than once (ISSUES.md D-171); these tests are
// about which groups a readerdef gets, not about that.
func activeNames(lines []string) []string {
	var names []string
	for _, l := range lines {
		if f := strings.Fields(l); len(f) > 0 {
			names = append(names, f[0])
		}
	}
	slices.Sort(names)
	return slices.Compact(names)
}

// TestReaderdefUnknownGroupList (D-164): a readerdef whose groups, listgroups or postgroups
// names a group list that dreader.access does not define makes the file invalid: it is not
// installed (logged), and on a first start dreaderd gives no service. Before, the name was
// looked up at login, not found, and the first group list ("everything") was used, so a typo
// gave the clients every group. A known name still works, and the readerdef default (no
// setting, "DEFAULT") still falls back to the first list.
func TestReaderdefUnknownGroupList(t *testing.T) {
	for _, opt := range []string{"groups", "listgroups", "postgroups"} {
		t.Run(opt, func(t *testing.T) {
			s := NewReaderSite(t, ReaderOptions{SpoolAddr: "127.0.0.1:1", Access: groupListAccess(opt + " testonlx"),
				NoStart: true, Config: map[string]string{"path_generallog": "%s/general.log"}})
			startReaderNoWait(t, s)
			var log string
			waitFor(t, 5*time.Second, "the unknown group list in the log", func() bool {
				log = generalLog(s)
				return strings.Contains(log, "testonlx")
			})
			if !strings.Contains(log, "not installed") && !strings.Contains(log, "not updating") {
				t.Errorf("the log does not say the file was refused:\n%s", log)
			}
			if g := readerGreeting(s.Addr); strings.HasPrefix(g, "200") || strings.HasPrefix(g, "201") {
				t.Errorf("dreaderd with %q in dreader.access gave service: %q", opt+" testonlx", strings.TrimSpace(g))
			}
		})
	}
	t.Run("known name", func(t *testing.T) {
		s := NewReaderSite(t, ReaderOptions{SpoolAddr: "127.0.0.1:1", Access: groupListAccess("groups testonly")})
		c := s.Reader(t)
		expect(t, c, 215, "LIST ACTIVE")
		names := activeNames(c.ReadDotLines())
		expect(t, c, 205, "QUIT")
		if !slices.Equal(names, []string{"e2e.test"}) {
			t.Errorf("LIST ACTIVE with groups testonly: %v, want [e2e.test]", names)
		}
	})
	t.Run("default", func(t *testing.T) {
		s := NewReaderSite(t, ReaderOptions{SpoolAddr: "127.0.0.1:1", Access: groupListAccess("read yes")})
		c := s.Reader(t)
		expect(t, c, 215, "LIST ACTIVE")
		n := len(activeNames(c.ReadDotLines()))
		expect(t, c, 205, "QUIT")
		if n != 2 {
			t.Errorf("LIST ACTIVE without a groups setting: %d groups, want 2 (the first list, everything)", n)
		}
	})
}

// TestReaderdefUnknownOption (D-166): a misspelled readerdef option makes dreader.access
// invalid: it is logged and the file is not installed, as for a bad number (D-161). Before,
// the option was logged as invalid and the rest of the file installed, so the readerdef ran
// without the setting the admin meant. Every known option is still accepted: xzver used to
// fall through to "Invalid readerdef option" after it was set.
func TestReaderdefUnknownOption(t *testing.T) {
	t.Run("misspelled", func(t *testing.T) {
		s := NewReaderSite(t, ReaderOptions{SpoolAddr: "127.0.0.1:1", Access: accessWith("maxconperhost 1"),
			NoStart: true, Config: map[string]string{"path_generallog": "%s/general.log"}})
		startReaderNoWait(t, s)
		var log string
		waitFor(t, 5*time.Second, "the invalid option in the log", func() bool {
			log = generalLog(s)
			return strings.Contains(log, "maxconperhost")
		})
		if !strings.Contains(log, "not updating") {
			t.Errorf("the log does not say the file was refused:\n%s", log)
		}
		if g := readerGreeting(s.Addr); strings.HasPrefix(g, "200") || strings.HasPrefix(g, "201") {
			t.Errorf("dreaderd with a misspelled readerdef option gave service: %q", strings.TrimSpace(g))
		}
	})
	t.Run("xzver", func(t *testing.T) {
		s := NewReaderSite(t, ReaderOptions{SpoolAddr: "127.0.0.1:1", Access: accessWith("xzver 1"),
			Config: map[string]string{"path_generallog": "%s/general.log"}})
		if g := readerGreeting(s.Addr); !strings.HasPrefix(g, "200") && !strings.HasPrefix(g, "201") {
			t.Errorf("dreaderd with xzver in a readerdef answers %q, want service", g)
		}
		if log := generalLog(s); strings.Contains(log, "Invalid readerdef option") {
			t.Errorf("xzver was logged as an invalid option:\n%s", log)
		}
	})
}

// TestReaderAccessCacheFileMissing (D-165): when one of the five access cache files cannot be
// opened while the main dreaderd process reloads the cache, it logs that and keeps the access
// map it has. Before, the main process exited ("Fatal: Unable to load access cache") on the
// next connection, and with it the whole reader.
func TestReaderAccessCacheFileMissing(t *testing.T) {
	s := NewReaderSite(t, ReaderOptions{SpoolAddr: "127.0.0.1:1",
		Config: map[string]string{"path_generallog": "%s/general.log"}})
	if g := readerGreeting(s.Addr); !strings.HasPrefix(g, "200") && !strings.HasPrefix(g, "201") {
		t.Fatalf("no service before the change: %q", g)
	}
	if err := os.Remove(filepath.Join(s.Dir, "cache.groupdef")); err != nil {
		t.Fatal(err)
	}
	// a newer lock makes the processes load the caches again
	future := time.Now().Add(time.Hour)
	if err := os.Chtimes(filepath.Join(s.Dir, ".accesslock"), future, future); err != nil {
		t.Fatal(err)
	}
	var log string
	waitFor(t, 10*time.Second, "the missing cache file in the log", func() bool {
		readerGreeting(s.Addr)
		log = generalLog(s)
		return strings.Contains(log, "Unable to open") && strings.Contains(log, "cache.groupdef")
	})
	for i := 0; i < 3; i++ {
		if g := readerGreeting(s.Addr); !strings.HasPrefix(g, "200") && !strings.HasPrefix(g, "201") {
			t.Fatalf("connection %d after the failed reload: %q, want the old access map to serve\n%s", i, g, generalLog(s))
		}
	}
	if strings.Contains(generalLog(s), "Fatal: Unable to load access cache") {
		t.Errorf("dreaderd gave up on the failed reload:\n%s", generalLog(s))
	}
}

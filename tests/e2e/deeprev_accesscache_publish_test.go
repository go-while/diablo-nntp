package e2e

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	testkit "github.com/go-while/nntp-testkit"
)

// The access cache tests that need both halves: the install that removes (or empties) the
// lock file when it cannot leave one consistent set, and the readers that load nothing from a
// lock file that is empty, gone or cannot be looked at. accessFaultSite and the fault
// injector are in deeprev_accesscache_set_test.go.

// TestDeepRevAccessCacheRemovedWhenNotOneSet (third and fourth review): when putting back the
// files of a failed install fails too, no process may load the five files as one set. The
// removals were not checked: when they failed as well, a whole mixed set stayed next to the
// lock file. The lock file is removed first now (without it nothing is loaded) and every
// failed removal is logged. The install that the main process retries on the next pass
// creates the lock file again, empty: only a completed install writes its stamp, and a reader
// loads nothing from an empty lock file. Before that, a retry that stopped before the renames
// left a fresh lock file next to the mixed set, and the readers loaded it.
//
// A preloaded library refuses the rename of cache.readeraccess.new, the put-back rename of
// cache.readerdef.old and the unlink of each of the five files; then also the link of
// cache.vserverdef, so that the retries stop before any rename. The lock file must be missing
// or empty, the running client keeps A; once the faults are gone, B is installed.
func TestDeepRevAccessCacheRemovedWhenNotOneSet(t *testing.T) {
	linkFlag := filepath.Join(t.TempDir(), "linkfail")
	s, flag, mark, list := accessFaultSite(t, "ACFAULT_RENAME=/cache.readeraccess.new,/cache.readerdef.old",
		"ACFAULT_UNLINK=/cache.vserverdef,/cache.groupdef,/cache.authdef,/cache.readerdef,/cache.readeraccess",
		"ACFAULT_LINK=/cache.vserverdef", "ACFAULT_LINKFLAG="+linkFlag)
	if err := os.WriteFile(flag, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	s.WriteFile(t, "dreader.access", accessSetB)
	testkit.WaitFor(t, 15*time.Second, "the install of B whose put-back fails", func() bool {
		return len(shimMarked(mark, "refused rename of")) >= 2 && len(shimMarked(mark, "refused unlink")) >= 5 &&
			strings.Contains(generalLog(s), "access cache files removed")
	})
	log := generalLog(s)
	if !strings.Contains(log, "Unable to remove "+filepath.Join(s.Dir, "cache.readerdef")) {
		t.Errorf("the failed removal of cache.readerdef is not in the log")
	}
	// the retries stop before any rename from now on, each with a new lock file
	if err := os.WriteFile(linkFlag, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	testkit.WaitFor(t, 15*time.Second, "a retry that stops at the link", func() bool {
		return len(shimMarked(mark, "refused link")) > 0
	})
	time.Sleep(time.Second)
	if fi, err := os.Stat(filepath.Join(s.Dir, ".accesslock")); err == nil && fi.Size() != 0 {
		t.Errorf("the lock file has a stamp (%d bytes) next to a set that is not one", fi.Size())
	}
	for i := 0; i < 3; i++ { // the reader and the DNS worker both load the cache
		if got := list(); !slices.Equal(got, []string{"e2e.test"}) {
			t.Fatalf("after the failed install and a retry that stopped before the renames: groups %q, want A's [e2e.test]: "+
				"a process loaded the mixed set", got)
		}
	}
	waitB(t, flag, list, linkFlag)
}

// TestDeepRevAccessCacheLockNotRemovable (fourth review): as above, but the lock file cannot be
// removed either. It is emptied then, and an empty lock file is not loaded: a dreaderd started
// now must not serve the mixed set (it has no access cache and refuses the client). Before,
// the lock file kept the stamp of the old set and a new process loaded the mixed one.
func TestDeepRevAccessCacheLockNotRemovable(t *testing.T) {
	s, flag, mark, _ := accessFaultSite(t, "ACFAULT_RENAME=/cache.readeraccess.new,/cache.readerdef.old",
		"ACFAULT_UNLINK=/cache.vserverdef,/cache.groupdef,/cache.authdef,/cache.readerdef,/cache.readeraccess,/.accesslock")
	if err := os.WriteFile(flag, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	s.WriteFile(t, "dreader.access", accessSetB)
	testkit.WaitFor(t, 15*time.Second, "the install of B whose put-back and removals fail", func() bool {
		return len(shimMarked(mark, "refused unlink of "+filepath.Join(s.Dir, ".accesslock"))) > 0
	})
	time.Sleep(time.Second)
	fi, err := os.Stat(filepath.Join(s.Dir, ".accesslock"))
	if err != nil {
		t.Fatalf("the lock file whose removal was refused is gone: %v", err)
	}
	if fi.Size() != 0 {
		t.Errorf("the lock file that could not be removed keeps its stamp (%d bytes) next to a set that is not one", fi.Size())
	}

	// a dreaderd started now (the faults stay) has no map: it must not serve the mixed set
	s.Proc.Stop()
	startReaderArgs(t, s)
	time.Sleep(2 * time.Second)
	c, err := net.DialTimeout("tcp", s.Addr, 5*time.Second)
	if err != nil {
		t.Fatalf("dial the restarted dreaderd: %v", err)
	}
	defer c.Close()
	c.SetDeadline(time.Now().Add(10 * time.Second))
	r := bufio.NewReader(c)
	g, _ := r.ReadString('\n')
	t.Logf("the restarted dreaderd answers %q", strings.TrimSpace(g))
	if g == "" {
		t.Fatalf("the restarted dreaderd gave no answer at all")
	}
	if strings.HasPrefix(g, "20") {
		fmt.Fprintf(c, "LIST ACTIVE\r\n")
		var lines []string
		for {
			l, err := r.ReadString('\n')
			if err != nil || strings.TrimSpace(l) == "." {
				break
			}
			lines = append(lines, l)
		}
		t.Errorf("a dreaderd started after the failed install served the client (%q): it loaded the set that is not one: %q",
			strings.TrimSpace(g), lines)
	}
}

// TestDeepRevAccessCacheLockFstatFails (fourth review): ReadAccessCache() takes the stamp of
// the lock file again with fstat() once it holds the shared lock. When that failed it went on
// and loaded the files; it loads nothing now (the process keeps its map).
//
// While a preloaded fstat() fails for .accesslock, B is installed: the client keeps A; once
// fstat works again, B.
func TestDeepRevAccessCacheLockFstatFails(t *testing.T) {
	fstatFlag := filepath.Join(t.TempDir(), "fstatfail")
	s, _, mark, list := accessFaultSite(t, "ACFAULT_FSTAT=/.accesslock", "ACFAULT_FSTATFLAG="+fstatFlag)
	if err := os.WriteFile(fstatFlag, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	s.WriteFile(t, "dreader.access", accessSetB)
	testkit.WaitFor(t, 15*time.Second, "B installed", func() bool {
		return strings.Count(generalLog(s), "Installing access cache") >= 2
	})
	time.Sleep(time.Second)
	for i := 0; i < 3; i++ {
		got := list()
		if len(shimMarked(mark, "refused fstat")) == 0 {
			t.Fatalf("no process looked at the lock file with fstat() through the injector (groups %q)", got)
		}
		if !slices.Equal(got, []string{"e2e.test"}) {
			t.Fatalf("with fstat() of the lock file failing: groups %q, want A's [e2e.test] (nothing loaded)", got)
		}
	}
	waitB(t, fstatFlag, list)
}

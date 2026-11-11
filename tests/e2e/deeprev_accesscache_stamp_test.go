package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

// TestReaderAccessCacheStampDiffers: dreaderd's processes load the access cache again when
// the access lock file changes; the master writes that file after it installs a new cache.
// They loaded it only when the file's time was later than the time of the last load, in
// whole seconds. After the clock stepped back, every later install had an older time and
// was not loaded until the clock caught up or dreaderd restarted, and a second install in
// the same second was missed. They now load it when its time differs in any way.
//
// The test puts the lock file's time an hour ahead, lets dreaderd load it, then installs a
// dreader.access that refuses 127.0.0.1: the new rules must apply. Then a same-second case:
// the lock file's time is set to a whole second and loaded, the rules are switched back, and
// once installed the time is set to the same second plus half a second: 127.0.0.1 must be
// served again.
func TestReaderAccessCacheStampDiffers(t *testing.T) {
	_, spool := spoolStore(t)
	s := NewReaderSite(t, ReaderOptions{Spool: spool})
	if l := firstLine(t, s.Addr); !strings.HasPrefix(l, "20") {
		t.Fatalf("greeting: %q", l)
	}
	lock := filepath.Join(s.Dir, ".accesslock")
	ino := func() uint64 {
		fi, err := os.Stat(filepath.Join(s.Dir, "cache.readeraccess"))
		if err != nil {
			t.Fatal(err)
		}
		return uint64(fi.Sys().(*syscall.Stat_t).Ino)
	}
	// the lock file's time an hour ahead, loaded by the reader's processes
	stampAccessLock(t, s, lock, time.Now().Add(time.Hour).Truncate(time.Second))
	// install waits until the master installed the new dreader.access (the cache files are
	// renamed over the old ones); no connection is made meanwhile, so nothing reloads
	install := func(access string) {
		t.Helper()
		before := ino()
		s.WriteFile(t, "dreader.access", access)
		deadline := time.Now().Add(15 * time.Second)
		for ino() == before {
			if time.Now().After(deadline) {
				t.Fatalf("the master did not install the new dreader.access:\n%s", generalLog(s))
			}
			time.Sleep(100 * time.Millisecond)
		}
	}
	// expect waits until a connection's first line starts with "20" (want) or not
	expect := func(served bool, why string) {
		t.Helper()
		deadline := time.Now().Add(10 * time.Second)
		for {
			l := firstLine(t, s.Addr)
			if strings.HasPrefix(l, "20") == served {
				return
			}
			if time.Now().After(deadline) {
				t.Fatalf("%s: last first line %q", why, l)
			}
			time.Sleep(200 * time.Millisecond)
		}
	}

	refuse := strings.Replace(DefaultReaderAccess,
		"readerdef rp\n    read yes\n    post yes\n", "readerdef rp\n    read no\n    post no\n", 1)
	install(refuse)
	if fi, err := os.Stat(lock); err != nil || !fi.ModTime().Before(time.Now().Add(time.Minute)) {
		t.Fatalf("the install did not write the lock file (%v, %v)", fi.ModTime(), err)
	}
	expect(false, "the new dreader.access (127.0.0.1 refused) was not loaded after the lock file's time had been later")

	// the same second, half a second later
	sec := time.Now().Add(2 * time.Hour).Truncate(time.Second)
	stampAccessLock(t, s, lock, sec)
	expect(false, "refused rules after the lock file's time changed")
	install(DefaultReaderAccess)
	half := sec.Add(500 * time.Millisecond)
	if err := os.Chtimes(lock, half, half); err != nil {
		t.Fatal(err)
	}
	if fi, err := os.Stat(lock); err != nil || fi.ModTime().Nanosecond() == 0 {
		t.Skipf("the file system keeps whole seconds only (%v): the same-second case is not testable", err)
	}
	expect(true, "the new dreader.access (127.0.0.1 served) was not loaded when the lock file's time changed within the same second")
}

// stampAccessLock sets the time of the access lock file and makes three connections: each makes
// the DNS process and the reader process of the site look at it.
func stampAccessLock(t *testing.T, s *Site, lock string, when time.Time) {
	t.Helper()
	if err := os.Chtimes(lock, when, when); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		firstLine(t, s.Addr)
		time.Sleep(100 * time.Millisecond)
	}
}

package e2e

// Tests of the internal spam filter's rate slots (D-281, D-282) and of a change of its cache
// size while feeder processes have the cache mapped (D-283). internalfilter B4: the 4th and
// later copies of one body within the slot's minutes are refused, so of n copies fed to a
// new slot 3 are stored.

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/go-while/nntp-testkit/client"
)

// spamTrip is the body filter's trip value in these tests; a new slot stores spamTrip-1
// copies of one body.
const spamTrip = 4

// spamSite starts a site with the internal body filter at internalfilter (e.g. "B4") on
// every group (label ISPAM).
func spamSite(t *testing.T, internalfilter string) *Site {
	t.Helper()
	return NewSite(t, SiteOptions{
		Config:    map[string]string{"internalfilter": internalfilter},
		Newsfeeds: DefaultNewsfeeds + "label ISPAM\n    addgroup *\nend\n",
	})
}

// TestSpamFilterSlowTransferKeepsTrip (D-281): a copy whose transfer began 13 s before the
// slot was made and that ends after the slot has tripped is refused, and so are the copies
// after it. Input: IHAVE of a copy, its head, 13 s pause; 5 copies of the body on another
// connection (3 stored, the slot trips); then the rest of the slow copy, then 3 more copies.
// Expected: the slow copy and the 3 copies after it are not stored. Fails before the fix:
// the spam check used the time the article began to come in, the slot time was more than
// 10 s after it, so the slot was reset and the slow copy and the 3 after it were stored.
func TestSpamFilterSlowTransferKeepsTrip(t *testing.T) {
	s := spamSite(t, "B4")
	const body = "identical body of a slow copy"

	slow := s.Peer(t)
	id := msgID(t)
	slow.ExpectCmd(335, "IHAVE %s", id)
	art := client.Stuff(newArticle(id, "e2e.test", "same body", body))
	cut := strings.Index(art, "\r\n\r\n") + 2 // the head and its blank line; the body later
	slow.MustSend(art[:cut])
	time.Sleep(13 * time.Second) // more than the 10 s a slot time may be ahead

	if got := feedSameBody(t, s, 5, body); got != spamTrip-1 {
		t.Fatalf("5 copies on a new slot: %d stored, want %d (the filter is not working in the test setup)", got, spamTrip-1)
	}

	slow.MustSend(art[cut:])
	code, line, err := slow.ReadStatus()
	if err != nil {
		t.Fatalf("IHAVE %s (slow copy): %v", id, err)
	}
	t.Logf("slow copy: %s", line)
	if c, _ := slow.Cmd("STAT %s", id); c == 223 {
		t.Errorf("the slow copy was stored (IHAVE reply %d) although its slot had tripped: the slot was reset", code)
	}
	slow.ExpectCmd(205, "QUIT")

	if got := feedSameBody(t, s, 3, body); got != 0 {
		t.Errorf("3 copies after the slow one: %d stored, want 0 (the slot stays tripped)", got)
	}
}

// TestSpamFilterDecayRestartsFromNow (D-282): a tripped slot that has been quiet for 58
// minutes (less than its one-hour expiry) starts again like a new one. Input: 6 copies of a
// body (3 stored, the slot trips), the slot's time moved back by 3500 s in spam.body.cache,
// then 10 more copies. Expected: 3 of the 10 stored, as for a new slot. Fails before the fix:
// the decay stopped at 0 but kept the idle minutes as credit, one more per copy, so all 10
// were stored (about 52 copies would pass before the slot trips again).
func TestSpamFilterDecayRestartsFromNow(t *testing.T) {
	if runtime.GOOS == "openbsd" {
		// The test writes the slot time with pwrite(); diablo reads it through a shared map,
		// which does not see it on OpenBSD (D-171).
		t.Skip("OpenBSD: a write() is not seen through other processes' shared maps (D-171)")
	}
	s := spamSite(t, "B4")
	const body = "identical body after a quiet hour"

	from := time.Now().Unix()
	before := feedSameBody(t, s, 6, body)
	to := time.Now().Unix()
	if before != spamTrip-1 {
		t.Fatalf("6 copies on a new slot: %d stored, want %d (the filter is not working in the test setup)", before, spamTrip-1)
	}

	ageSpamSlot(t, filepath.Join(s.Dir, "spam.body.cache"), from, to, 3500)

	if got := feedSameBody(t, s, 10, body); got != before {
		t.Errorf("10 copies after 58 quiet minutes: %d stored, want %d (as on a new slot)", got, before)
	}
}

// ageSpamSlot moves the time of the one used slot of the spam cache file back by secs: it
// looks for the one aligned time_t (64 or 32 bits, native byte order) between from and to.
// Diablo reads the file through its shared map, so the change is seen at the next copy.
func ageSpamSlot(t *testing.T, path string, from, to, secs int64) {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	in := func(v int64) bool { return v >= from-1 && v <= to+1 }
	var offs []int
	size := 8
	for off := 0; off+8 <= len(b); off += 8 {
		if in(int64(binary.NativeEndian.Uint64(b[off:]))) {
			offs = append(offs, off)
		}
	}
	if len(offs) == 0 { // 32-bit time_t
		size = 4
		for off := 0; off+4 <= len(b); off += 4 {
			if in(int64(int32(binary.NativeEndian.Uint32(b[off:])))) {
				offs = append(offs, off)
			}
		}
	}
	if len(offs) != 1 {
		t.Fatalf("%s: %d values between %d and %d, want the one slot time", path, len(offs), from, to)
	}
	off := offs[0]
	v := make([]byte, size)
	if size == 8 {
		binary.NativeEndian.PutUint64(v, binary.NativeEndian.Uint64(b[off:])-uint64(secs))
	} else {
		binary.NativeEndian.PutUint32(v, binary.NativeEndian.Uint32(b[off:])-uint32(secs))
	}
	f, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := f.WriteAt(v, int64(off)); err != nil {
		t.Fatal(err)
	}
}

// TestSpamFilterResizeKeepsMappedCache (D-283): a new cache size leaves the feeder processes
// that have the old cache mapped working. Input: a peer connection open (its feeder process
// has the 65536-entry cache mapped), "dicmd config internalfilter B4s1024", then 4 articles
// with different bodies on that connection, then 8 copies of one body on a new one.
// Expected: spam.body.cache has 1024 entries, the 4 articles get 235, and the new connection
// stores 3 of the 8 copies (the filter works at the new size). Fails before the fix: the
// cache file was truncated and rewritten at 1024 entries under the old process's map, which
// got SIGBUS at the first slot past the new end, so the connection was closed.
func TestSpamFilterResizeKeepsMappedCache(t *testing.T) {
	s := spamSite(t, "B4")
	cache := filepath.Join(s.Dir, "spam.body.cache")
	st, err := os.Stat(cache)
	if err != nil {
		t.Fatal(err)
	}
	oldSize := st.Size()

	old := s.Peer(t) // forked with the cache mapped at 65536 entries

	out := s.Dicmd(t, "config", "internalfilter", "B4s1024")
	if !strings.Contains(out, "Set 'internalfilter' to 'B4s1024'") {
		t.Fatalf("dicmd config internalfilter B4s1024: %q", out)
	}
	if st, err = os.Stat(cache); err != nil {
		t.Fatal(err)
	}
	if st.Size()*64 != oldSize {
		t.Errorf("spam.body.cache is %d bytes after the change, want %d (1024 of %d-byte entries)", st.Size(), oldSize/64, oldSize/65536)
	}

	for i := 0; i < 4; i++ {
		id := msgID(t)
		code, line, err := old.TryCmd("IHAVE %s", id)
		if err == nil && code == 335 {
			if err = old.Send(client.Stuff(newArticle(id, "e2e.test", "after the size change", "body number", id))); err == nil {
				code, line, err = old.ReadStatus()
			}
		}
		if err != nil {
			t.Fatalf("article %d on the connection opened before the size change: %v (its feeder process is gone)", i+1, err)
		}
		if code != 235 {
			t.Errorf("article %d on the connection opened before the size change: %q, want 235", i+1, line)
		}
	}
	old.ExpectCmd(205, "QUIT")

	if got := feedSameBody(t, s, 8, "identical body after the size change"); got != spamTrip-1 {
		t.Errorf("8 copies on a new connection after the size change: %d stored, want %d", got, spamTrip-1)
	}
}

// TestSpamFilterResizeKeepsMode (D-283): the cache file written for a new size has the mode
// of the file it replaces. Input: spam.body.cache set to 0664 while diablo runs, then "dicmd
// config internalfilter B4s1024". Expected: spam.body.cache has 1024 entries and mode 0664.
// Fails on the first D-283 fix (#476 at 8e0c769f): the new file got 0644 (the create mode),
// so a cache whose owner or group is the daemon's could become one it cannot open. Passes
// on main 7e157f5c, which rewrote the file in place. The owner is kept the same way; that
// part needs root and is not tested here.
func TestSpamFilterResizeKeepsMode(t *testing.T) {
	s := spamSite(t, "B4")
	cache := filepath.Join(s.Dir, "spam.body.cache")
	st, err := os.Stat(cache)
	if err != nil {
		t.Fatal(err)
	}
	oldSize := st.Size()
	if err := os.Chmod(cache, 0o664); err != nil {
		t.Fatal(err)
	}

	out := s.Dicmd(t, "config", "internalfilter", "B4s1024")
	if !strings.Contains(out, "Set 'internalfilter' to 'B4s1024'") {
		t.Fatalf("dicmd config internalfilter B4s1024: %q", out)
	}
	if st, err = os.Stat(cache); err != nil {
		t.Fatal(err)
	}
	if st.Size()*64 != oldSize {
		t.Errorf("spam.body.cache is %d bytes after the change, want %d", st.Size(), oldSize/64)
	}
	if m := st.Mode().Perm(); m != 0o664 {
		t.Errorf("spam.body.cache has mode %04o after the size change, want 0664 (the mode it had)", m)
	}
}

package e2e

import (
	"encoding/binary"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/go-while/nntp-testkit/client"
)

// A feeder child holds an exclusive fcntl lock on the spool file (B.<iter>) it appends to,
// for as long as it keeps that file open. The lock is the only thing that keeps another
// child out of that file: a child whose search for a free B.<iter> reaches a locked file
// moves on to the next iter, but takes an unlocked one and appends to it at its own offset.
// POSIX drops all of a process's fcntl locks on a file when it closes any descriptor for
// that file, so the child must not open and close the file it writes, nor a dup() of its
// descriptor.
//
// The start iter of a connection comes from a hash of peer addresses that the diablo master
// adds every accepted connection's address to (PeerIpHash in util/diablo.c): as long as the
// sum is below 65536, the next connection starts at the previous one's iter plus the byte
// sum of its address. TestFeederFullSpoolFileKeepsLock relies on that to place a file where
// a connection will start.

// spoolFiles returns the spool files (B.<iter>) under the site's spool directory.
func spoolFiles(tb testing.TB, s *Site) []string {
	tb.Helper()
	var files []string
	err := filepath.WalkDir(filepath.Join(s.Dir, "spool"), func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.HasPrefix(d.Name(), "B.") {
			files = append(files, p)
		}
		return nil
	})
	if err != nil {
		tb.Fatal(err)
	}
	slices.Sort(files)
	return files
}

// spoolLockPid returns the pid of the process that holds a lock on the whole of path that
// conflicts with a write lock (F_GETLK), or 0 if nobody does.
func spoolLockPid(tb testing.TB, path string) int {
	tb.Helper()
	f, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		tb.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	lk := syscall.Flock_t{Type: syscall.F_WRLCK, Whence: 0, Start: 0, Len: 0}
	if err := syscall.FcntlFlock(f.Fd(), syscall.F_GETLK, &lk); err != nil {
		tb.Fatalf("F_GETLK %s: %v", path, err)
	}
	if lk.Type == syscall.F_UNLCK {
		return 0
	}
	return int(lk.Pid)
}

// fetchBody sends ARTICLE id and returns the article's body, or reports the reply and
// returns nil.
func fetchBody(tb testing.TB, c *client.Conn, id string) []string {
	tb.Helper()
	code, line := c.Cmd("ARTICLE %s", id)
	if code != 220 {
		tb.Errorf("ARTICLE %s: got %q, want 220", id, line)
		return nil
	}
	_, body := splitArticle(c.ReadDotLines())
	return body
}

// testSpoolLockKept stores an article, reads it back with ARTICLE, HEAD and BODY on the
// same connection and checks after each that the spool file is still locked by the feeder
// child. Then the connection stores a second article (reading through the descriptor the
// child appends with must not move where it appends), and a new connection reads both.
func testSpoolLockKept(t *testing.T, spoolCtl string, gzipped bool) {
	s := NewSite(t, SiteOptions{SpoolCtl: spoolCtl})

	c1 := s.Peer(t)
	a := msgID(t)
	expect(t, c1, 335, "IHAVE %s", a)
	send(t, c1, wire(newArticle(a, "e2e.test", "article a", "body of a")))
	status(t, c1, 235, "IHAVE a")

	files := spoolFiles(t, s)
	if len(files) != 1 {
		t.Fatalf("spool files after one article: %q, want one", files)
	}
	file := files[0]
	raw, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Contains(string(raw), "\x1f\x8b"); got != gzipped {
		t.Fatalf("gzip data in %s: %v, want %v", filepath.Base(file), got, gzipped)
	}
	owner := spoolLockPid(t, file)
	if owner == 0 {
		t.Errorf("%s is not locked after the first article was stored", filepath.Base(file))
	}

	for _, cmd := range []string{"ARTICLE", "HEAD", "BODY"} {
		code, line := c1.Cmd("%s %s", cmd, a)
		if code < 220 || code > 222 {
			t.Fatalf("%s %s: got %q, want 220-222", cmd, a, line)
		}
		c1.ReadDotLines()
		if pid := spoolLockPid(t, file); pid != owner {
			t.Errorf("after %s of an article in the file the connection appends to, the lock on %s is held by pid %d, want %d (the feeder child)",
				cmd, filepath.Base(file), pid, owner)
		}
	}

	b := msgID(t)
	bBody := []string{"body of b", strings.Repeat("b", 200)}
	expect(t, c1, 335, "IHAVE %s", b)
	send(t, c1, wire(newArticle(b, "e2e.test", "article b", bBody...)))
	status(t, c1, 235, "IHAVE b")
	if got := spoolFiles(t, s); !slices.Equal(got, files) {
		t.Errorf("spool files after the second article: %q, want %q", got, files)
	}

	c2 := s.Peer(t)
	if got := fetchBody(t, c2, a); got != nil && !slices.Equal(got, []string{"body of a"}) {
		t.Errorf("ARTICLE %s body %q, want %q", a, got, "body of a")
	}
	if got := fetchBody(t, c2, b); got != nil && !slices.Equal(got, bBody) {
		t.Errorf("ARTICLE %s body %q, want %q", b, got, bBody)
	}
	expect(t, c2, 205, "QUIT")
	expect(t, c1, 205, "QUIT")
}

// TestFeederArticleKeepsSpoolLock: ARTICLE, HEAD and BODY of an article in the spool file
// the connection is appending to leave the connection's lock on that file in place.
// **Fails** on main: ArticleOpen() opened the file again and closed it, which dropped the
// lock.
func TestFeederArticleKeepsSpoolLock(t *testing.T) {
	testSpoolLockKept(t, DefaultSpoolCtl, false)
}

// TestFeederCompressedSpoolKeepsLock: the same with a compressed spool (compresslvl), where
// storing and reading an article go through gzip. **Fails** on main: storing the article
// closed a dup() of the spool descriptor (gzclose()), so the lock was gone after the first
// article, before any ARTICLE.
func TestFeederCompressedSpoolKeepsLock(t *testing.T) {
	skipWithoutZlib(t)
	testSpoolLockKept(t, compressedSpoolCtl, true)
}

// spoolIhave offers and sends an article with IHAVE and expects 235.
func spoolIhave(tb testing.TB, c *client.Conn, id string, body ...string) {
	tb.Helper()
	expect(tb, c, 335, "IHAVE %s", id)
	send(tb, c, wire(newArticle(id, "e2e.test", "spool lock", body...)))
	status(tb, c, 235, "IHAVE "+id)
}

const compressedSpoolCtl = `spool 00
    minfree 1m
    compresslvl 6
end
metaspool trad
    spool 00
end
expire * trad
`

// TestFeederCorruptSpoolHeader: a compressed article whose spool header has a bad length
// (ArtLen far too large, so large that the buffer size wraps, or not matching the history
// entry; HeadLen not the header's size) is refused with 430 and logged, without sizing a
// buffer from it, and the connection keeps serving. **Fails** on main and before this fix:
// no check, so the buffer size wrapped (ArtLen 0xfffffff0 allocated 10 bytes and the
// article was inflated into it) or HeadLen copied past the header.
func TestFeederCorruptSpoolHeader(t *testing.T) {
	skipWithoutZlib(t)
	s := NewSite(t, SiteOptions{
		SpoolCtl: compressedSpoolCtl,
		Config:   map[string]string{"path_generallog": "@home@/log/general.log"},
	})
	c := s.Peer(t)
	a, b := msgID(t), msgID(t)
	aBody := []string{"body of a", strings.Repeat("hello world ", 50)}
	spoolIhave(t, c, a, aBody...)
	spoolIhave(t, c, b, "body of b")
	expect(t, c, 205, "QUIT")
	file := spoolFiles(t, s)[0]
	waitFor(t, 10*time.Second, "the child to let go of the spool file", func() bool { return spoolLockPid(t, file) == 0 })

	// a is the first article of the file: its spool header is at offset 0. Layout:
	// Magic1 Magic2 Version HeadLen StoreType, 3 bytes padding, ArtHdrLen, ArtLen,
	// StoreLen (host order uint32), HdrEnd.
	f, err := os.OpenFile(file, os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	hdr := make([]byte, 24)
	if _, err := f.ReadAt(hdr, 0); err != nil {
		t.Fatal(err)
	}
	if hdr[3] != 24 || hdr[4]&2 == 0 { // STORETYPE_GZIP 0x02
		t.Fatalf("spool header % x: want HeadLen 24 and the gzip store type", hdr)
	}
	artLen := binary.NativeEndian.Uint32(hdr[12:])
	for _, tc := range []struct {
		name    string
		off     int64
		val     []byte
		logline string
	}{
		{"ArtLen wraps", 12, binary.NativeEndian.AppendUint32(nil, 0xfffffff0), "artlen 4294967280"},
		{"ArtLen 2 GB", 12, binary.NativeEndian.AppendUint32(nil, 0x80000000), "artlen 2147483648"},
		{"ArtLen off by one", 12, binary.NativeEndian.AppendUint32(nil, artLen+1), "artlen " + strconv.Itoa(int(artLen+1))},
		{"HeadLen 200", 3, []byte{200}, "headlen 200"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := f.WriteAt(tc.val, tc.off); err != nil {
				t.Fatal(err)
			}
			defer func() { _, _ = f.WriteAt(hdr, 0) }()
			c := s.Peer(t)
			expect(t, c, 430, "ARTICLE %s", a)
			if got := fetchBody(t, c, b); got != nil && !slices.Equal(got, []string{"body of b"}) {
				t.Errorf("ARTICLE %s after the refused one: body %q", b, got)
			}
			expect(t, c, 205, "QUIT")
			if log := s.ReadFile(t, "log/general.log"); !strings.Contains(log, "corrupt spool header") || !strings.Contains(log, tc.logline) {
				t.Errorf("general.log has no \"corrupt spool header ... %s\" line:\n%s", tc.logline, log)
			}
		})
	}
	c = s.Peer(t)
	if got := fetchBody(t, c, a); got != nil && !slices.Equal(got, aBody) {
		t.Errorf("ARTICLE %s with its header restored: body %q", a, got)
	}
	expect(t, c, 205, "QUIT")
}

// TestFeederFullSpoolFileKeepsLock: when an article does not fit in the spool file a child
// appends to, the child goes on in a new file and keeps its lock on the full one. **Fails**
// on main: the search for the new file opened the full one again (fcntl locks never
// conflict within a process), found it full and closed it, which dropped the lock while the
// full file was still in the child's cache.
func TestFeederFullSpoolFileKeepsLock(t *testing.T) {
	const maxFile = 1 << 30 // SPOOL_MAX_FILE_SIZE
	s := NewSite(t, SiteOptions{})

	// The first connection shows where connections start now.
	c0 := s.Peer(t)
	spoolIhave(t, c0, msgID(t), "x")
	expect(t, c0, 205, "QUIT")
	f0 := spoolFiles(t, s)[0]
	waitFor(t, 10*time.Second, "the child to let go of the spool file", func() bool { return spoolLockPid(t, f0) == 0 })
	dir := filepath.Dir(f0)
	i0, err := strconv.ParseUint(strings.TrimPrefix(filepath.Base(f0), "B."), 16, 16)
	if err != nil {
		t.Fatal(err)
	}
	step := 0
	for _, ch := range []byte(PeerIP) {
		step += int(ch)
	}

	// The next connection starts at i0+step: put a nearly full file there (sparse).
	full := filepath.Join(dir, fmt.Sprintf("B.%04x", (int(i0)+step)&0x7fff))
	if err := os.WriteFile(full, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Truncate(full, maxFile-20000); err != nil {
		t.Fatal(err)
	}

	c1 := s.Peer(t)
	spoolIhave(t, c1, msgID(t), "small")
	if st, err := os.Stat(full); err != nil || st.Size() == maxFile-20000 {
		if slices.ContainsFunc(spoolFiles(t, s), func(f string) bool { return filepath.Dir(f) != dir }) {
			t.Skip("the spool directory changed (10 minute boundary)")
		}
		t.Fatalf("the article did not go to %s, where the connection was expected to start (spool files %q); see PeerIpHash in util/diablo.c",
			filepath.Base(full), spoolFiles(t, s))
	}
	owner := spoolLockPid(t, full)
	if owner == 0 {
		t.Fatalf("%s is not locked after an article was stored in it", filepath.Base(full))
	}

	var big []string
	for range 400 {
		big = append(big, strings.Repeat("b", 70))
	}
	before := spoolFiles(t, s)
	spoolIhave(t, c1, msgID(t), big...)
	if after := spoolFiles(t, s); len(after) != len(before)+1 {
		t.Errorf("spool files after an article that does not fit: %q, want one more than %q", after, before)
	}
	if pid := spoolLockPid(t, full); pid != owner {
		t.Errorf("after the child moved on to a new file, the lock on the full %s is held by pid %d, want %d (the feeder child)",
			filepath.Base(full), pid, owner)
	}
	expect(t, c1, 205, "QUIT")
}

// TestFeederSlotAliasKeepsLock: with reallocint 1m a child reopens its spool file under a
// new slot after a minute (the directory, D.<time>, still spans 10 minutes); ARTICLE of an
// article stored under the old slot reads through the cached descriptor and keeps the lock.
// **Fails** before this fix: the file was matched by slot and iter, not by inode, so it was
// opened and closed again. Takes about a minute (up to three).
func TestFeederSlotAliasKeepsLock(t *testing.T) {
	// The test needs two minutes of one spool directory: wait for the next one if fewer
	// are left.
	if now := time.Now(); now.Unix()/60%10 >= 8 {
		time.Sleep(time.Duration(600-now.Unix()%600)*time.Second + time.Second)
	}
	start := time.Now()
	s := NewSite(t, SiteOptions{SpoolCtl: `spool 00
    minfree 1m
end
metaspool trad
    reallocint 1m
    spool 00
end
expire * trad
`})
	c := s.Peer(t)
	a := msgID(t)
	spoolIhave(t, c, a, "body of a")
	files := spoolFiles(t, s)
	if len(files) != 1 {
		t.Fatalf("spool files after one article: %q, want one", files)
	}
	file := files[0]
	time.Sleep(62 * time.Second) // the child allocates spools again 60 s after it started
	if time.Now().Unix()/600 != start.Unix()/600 {
		t.Skip("crossed into a new spool directory")
	}
	spoolIhave(t, c, msgID(t), "body of b")
	if got := spoolFiles(t, s); !slices.Equal(got, files) {
		t.Fatalf("spool files after the second article: %q, want %q (the same file under the new slot)", got, files)
	}
	owner := spoolLockPid(t, file)
	if owner == 0 {
		t.Fatalf("%s is not locked after the second article", filepath.Base(file))
	}
	if got := fetchBody(t, c, a); got != nil && !slices.Equal(got, []string{"body of a"}) {
		t.Errorf("ARTICLE %s body %q", a, got)
	}
	if pid := spoolLockPid(t, file); pid != owner {
		t.Errorf("after ARTICLE of an article stored under the previous slot, the lock on %s is held by pid %d, want %d (the feeder child)",
			filepath.Base(file), pid, owner)
	}
	expect(t, c, 205, "QUIT")
}

// TestFeederCorruptGzipData: a compressed article must inflate to exactly its length and end
// with its gzip trailer, whose CRC and length zlib checks. A flipped CRC or length byte, or
// a member cut short at the end of the file, is refused with 430 and logged, and the
// connection keeps serving the intact article next to it. **Fails** before this fix: the
// article was served as soon as the output buffer was full, whatever inflate said about the
// rest (found by the third review, GPT Terra).
func TestFeederCorruptGzipData(t *testing.T) {
	skipWithoutZlib(t)
	s := NewSite(t, SiteOptions{
		SpoolCtl: compressedSpoolCtl,
		Config:   map[string]string{"path_generallog": "@home@/log/general.log"},
	})
	c := s.Peer(t)
	a, b := msgID(t), msgID(t)
	aBody := []string{"body of a", strings.Repeat("hello world ", 50)}
	bBody := []string{"body of b", strings.Repeat("goodbye world ", 50)}
	spoolIhave(t, c, a, aBody...)
	spoolIhave(t, c, b, bBody...)
	expect(t, c, 205, "QUIT")
	file := spoolFiles(t, s)[0]
	waitFor(t, 10*time.Second, "the child to let go of the spool file", func() bool { return spoolLockPid(t, file) == 0 })

	orig, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	// Each article: a 24-byte spool header, its gzip member, a NUL. StoreLen (offset 16 of
	// the header) is header plus member, so the member's 8-byte trailer (CRC32, then the
	// length) ends at StoreLen.
	aEnd := int64(binary.NativeEndian.Uint32(orig[16:]))
	if orig[24] != 0x1f || orig[25] != 0x8b || orig[aEnd] != 0 || orig[aEnd+1] != orig[0] {
		t.Fatalf("unexpected spool layout: % x ... StoreLen %d", orig[:28], aEnd)
	}
	bOff := aEnd + 1
	bEnd := bOff + int64(binary.NativeEndian.Uint32(orig[bOff+16:]))
	if bEnd+1 != int64(len(orig)) || orig[bEnd] != 0 {
		t.Fatalf("b's StoreLen does not end at the file's last byte (end %d, file %d)", bEnd, len(orig))
	}

	f, err := os.OpenFile(file, os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	flip := func(off int64) func() {
		return func() {
			if _, err := f.WriteAt([]byte{orig[off] ^ 1}, off); err != nil {
				t.Fatal(err)
			}
		}
	}
	for _, tc := range []struct {
		name     string
		damage   func()
		bad      string
		good     string
		goodBody []string
	}{
		{"a's CRC", flip(aEnd - 8), a, b, bBody},
		{"a's length", flip(aEnd - 4), a, b, bBody},
		{"b cut in its trailer", func() {
			if err := f.Truncate(bEnd - 3); err != nil {
				t.Fatal(err)
			}
		}, b, a, aBody},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tc.damage()
			defer func() {
				if _, err := f.WriteAt(orig, 0); err != nil {
					t.Fatal(err)
				}
			}()
			c := s.Peer(t)
			code, line := c.Cmd("ARTICLE %s", tc.bad)
			if code == 220 {
				c.ReadDotLines()
			}
			if code != 430 {
				t.Errorf("ARTICLE %s with %s damaged: got %q, want 430", tc.bad, tc.name, line)
			}
			if got := fetchBody(t, c, tc.good); got != nil && !slices.Equal(got, tc.goodBody) {
				t.Errorf("ARTICLE %s next to the damaged one: body %q", tc.good, got)
			}
			expect(t, c, 205, "QUIT")
			if log := s.ReadFile(t, "log/general.log"); !strings.Contains(log, "Error uncompressing article") {
				t.Errorf("general.log has no \"Error uncompressing article\" line:\n%s", log)
			}
		})
	}
	c = s.Peer(t)
	for _, x := range []struct {
		id   string
		body []string
	}{{a, aBody}, {b, bBody}} {
		if got := fetchBody(t, c, x.id); got != nil && !slices.Equal(got, x.body) {
			t.Errorf("ARTICLE %s with the file restored: body %q", x.id, got)
		}
	}
	expect(t, c, 205, "QUIT")
}

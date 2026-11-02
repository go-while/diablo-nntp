package e2e

import (
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"testing"

	"github.com/go-while/nntp-testkit/client"
)

// A feeder child holds an exclusive fcntl lock on the spool file (B.<iter>) it appends to,
// for as long as it keeps that file open. The lock is the only thing that keeps a second
// child, which starts on the same B.<iter> when its peer has the same address, from
// appending to the same file. POSIX drops all of a process's fcntl locks on a file when it
// closes any descriptor for that file, so the child must not open and close the file it
// writes, nor a dup() of its descriptor.

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
	testSpoolLockKept(t, `spool 00
    minfree 1m
    compresslvl 6
end
metaspool trad
    spool 00
end
expire * trad
`, true)
}

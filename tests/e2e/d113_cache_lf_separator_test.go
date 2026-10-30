package e2e

import (
	"bufio"
	"fmt"
	"io/fs"
	"net"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/go-while/nntp-testkit"
)

// rawReader is a plain client of dreaderd that, unlike nntp-testkit's, accepts bare LF line
// ends in a multi-line reply: the cache hands the lines of a cached article on as stored.
type rawReader struct {
	t *testing.T
	c net.Conn
	r *bufio.Reader
}

func dialRaw(t *testing.T, addr string) *rawReader {
	t.Helper()
	c, err := net.DialTimeout("tcp", addr, testkit.Deadline())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Close() })
	rr := &rawReader{t: t, c: c, r: bufio.NewReader(c)}
	rr.line() // greeting
	return rr
}

// line reads one line and returns it without its CR/LF.
func (rr *rawReader) line() string {
	rr.t.Helper()
	rr.c.SetReadDeadline(time.Now().Add(testkit.Deadline()))
	s, err := rr.r.ReadString('\n')
	if err != nil {
		rr.t.Fatalf("read: %v (got %q)", err, s)
	}
	return strings.TrimRight(s, "\r\n")
}

// article sends cmd (ARTICLE, HEAD or BODY with the id) and returns the lines of the reply up
// to the "." line, with their line ends removed. The status must be wantCode.
func (rr *rawReader) article(cmd, id string, wantCode string) []string {
	rr.t.Helper()
	fmt.Fprintf(rr.c, "%s %s\r\n", cmd, id)
	st := rr.line()
	if !strings.HasPrefix(st, wantCode) {
		rr.t.Fatalf("%s %s: %q, want %s", cmd, id, st, wantCode)
	}
	var lines []string
	for l := rr.line(); l != "."; l = rr.line() {
		lines = append(lines, l)
	}
	return lines
}

// cachedFile returns the one non-empty file of the article cache.
func cachedFile(t *testing.T, dir string) string {
	t.Helper()
	var found []string
	filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err == nil && d.Type().IsRegular() {
			if fi, err := d.Info(); err == nil && fi.Size() > 0 {
				found = append(found, p)
			}
		}
		return nil
	})
	if len(found) != 1 {
		t.Fatalf("want one cached article in %s, found %q", dir, found)
	}
	return found[0]
}

// TestCachedLFSeparator (D-113): the cache dump ended the header only at a CRLF blank line. In
// an article stored with LF line ends the blank line is a bare LF: it never ended the header,
// so a cached HEAD printed the whole article and BODY nothing. The article is read once
// (which fills the cache), its cached HEAD and BODY are taken, then the cache file is
// rewritten with LF line ends; HEAD and BODY from it must have the same lines as before.
//
// A spool server cannot deliver such an article into the cache today (dreaderd answers 430 when
// it never sees the CRLF blank line), so the test rewrites the cache file itself.
func TestCachedLFSeparator(t *testing.T) {
	st, spool := spoolStore(t)
	s := NewReaderSite(t, ReaderOptions{Spool: spool, Config: map[string]string{"readercache": "on"}})
	id := msgID(t)
	feedReader(t, s, st, "e2e.test", newArticle(id, "e2e.test", "lf only", "body one", "body two"))

	r := s.Reader(t)
	expect(t, r, 220, "ARTICLE %s", id) // from the spool; fills the cache
	r.ReadDotLines()
	cacheDir := filepath.Join(s.Dir, "spool", "cache")
	waitFor(t, 10*time.Second, "the cached article in "+cacheDir, func() bool {
		return countNonEmptyFiles(cacheDir) >= 1
	})
	expect(t, r, 205, "QUIT")

	cached := dialRaw(t, s.Addr)
	wantHead := cached.article("HEAD", id, "221")
	wantBody := cached.article("BODY", id, "222")
	wantArt := cached.article("ARTICLE", id, "220")
	if len(wantHead) < 5 || !slices.Equal(wantBody, []string{"body one", "body two"}) {
		t.Fatalf("cached reply from the CRLF cache is wrong:\nHEAD %q\nBODY %q", wantHead, wantBody)
	}

	f := cachedFile(t, cacheDir)
	data, err := os.ReadFile(f)
	if err != nil {
		t.Fatal(err)
	}
	lf := strings.ReplaceAll(string(data), "\r\n", "\n")
	if lf == string(data) || !strings.Contains(lf, "\n\nbody one\n") {
		t.Fatalf("cache file %s has no CRLF line ends to convert:\n%q", f, data)
	}
	if err := os.WriteFile(f, []byte(lf), 0o644); err != nil {
		t.Fatal(err)
	}

	lfr := dialRaw(t, s.Addr)
	if got := lfr.article("HEAD", id, "221"); !slices.Equal(got, wantHead) {
		t.Errorf("HEAD from an LF-only cached article:\n got %q\nwant %q", got, wantHead)
	}
	if got := lfr.article("BODY", id, "222"); !slices.Equal(got, wantBody) {
		t.Errorf("BODY from an LF-only cached article:\n got %q\nwant %q", got, wantBody)
	}
	if got := lfr.article("ARTICLE", id, "220"); !slices.Equal(got, wantArt) {
		t.Errorf("ARTICLE from an LF-only cached article:\n got %q\nwant %q", got, wantArt)
	}
}

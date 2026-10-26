package e2e

import (
	"io/fs"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-while/nntp-testkit/fakeserver"
)

// TestReaderCacheHeaderRewrite: with readercache on, a repeated HEAD/ARTICLE is served from
// dreaderd's article cache, which puts the vserver's cluster name (PathHost) in Xref: in
// place of the spool's host and in front of Path:. An Xref: with a host but no groups and an
// empty Path: are passed on unchanged, and the header lines after them are not touched.
// Before, the rewrite of those two lines did not stop at the end of the line and copied the
// following headers (and whatever came after the article in memory) into it.
func TestReaderCacheHeaderRewrite(t *testing.T) {
	st, spool := spoolStore(t)
	s := NewReaderSite(t, ReaderOptions{
		Spool:  spool,
		Config: map[string]string{"readercache": "on"},
	})

	type tc struct {
		name               string
		xref, path         string // header lines in the spool's copy
		wantXref, wantPath string // the same lines as served from the cache
	}
	cases := []tc{
		{"well-formed", "Xref: e2e.spool e2e.test:1", "Path: e2e.spool!not-for-mail",
			"Xref: " + PathHost + " e2e.test:1", "Path: " + PathHost + "!e2e.spool!not-for-mail"},
		{"xref without groups", "Xref: e2e.spool", "Path: e2e.spool!not-for-mail",
			"Xref: e2e.spool", "Path: " + PathHost + "!e2e.spool!not-for-mail"},
		{"empty path", "Xref: e2e.spool e2e.test:3", "Path:",
			"Xref: " + PathHost + " e2e.test:3", "Path:"},
	}

	ids := make([]string, len(cases))
	heads := make([][]string, len(cases))
	p := s.dialFrom(t, PeerIP)
	for i, c := range cases {
		id := msgID(t)
		ids[i] = id
		art := newArticle(id, "e2e.test", "cache rewrite "+c.name, "body of "+c.name)
		head, body := splitArticle(art)
		// The spool's copy: Xref: first, then Path: and the rest, as a spool server sends it.
		sh := []string{c.xref, c.path}
		for _, h := range head {
			if !strings.HasPrefix(h, "Path:") {
				sh = append(sh, h)
			}
		}
		heads[i] = sh
		st.AddArticle("e2e.test", fakeserver.Article{MessageID: id, Headers: sh, Body: body})
		expect(t, p, 335, "IHAVE %s", id)
		send(t, p, wire(art))
		status(t, p, 235, "IHAVE "+id)
	}
	expect(t, p, 205, "QUIT")

	r := s.Reader(t)
	waitFor(t, 10*time.Second, "the articles in GROUP e2e.test", func() bool {
		_, line := r.Cmd("GROUP e2e.test")
		f := strings.Fields(line)
		return len(f) > 1 && f[0] == "211" && f[1] == "3"
	})
	// First read: fetched from the spool, which fills the cache.
	for _, id := range ids {
		expect(t, r, 220, "ARTICLE %s", id)
		r.ReadDotLines()
	}
	cacheDir := filepath.Join(s.Dir, "spool", "cache")
	waitFor(t, 10*time.Second, "three cached articles in "+cacheDir, func() bool {
		return countNonEmptyFiles(cacheDir) >= len(ids)
	})

	// Second read: from the cache.
	for i, c := range cases {
		expect(t, r, 221, "HEAD %s", ids[i])
		got := r.ReadDotLines()
		checkCachedHead(t, c.name, got, heads[i], c.wantXref, c.wantPath)

		expect(t, r, 220, "ARTICLE %s", ids[i])
		got = r.ReadDotLines()
		h, b := splitArticle(got)
		checkCachedHead(t, c.name+" (ARTICLE)", h, heads[i], c.wantXref, c.wantPath)
		if len(b) != 1 || b[0] != "body of "+c.name {
			t.Errorf("%s: ARTICLE body %q", c.name, b)
		}
	}
	expect(t, r, 205, "QUIT")

	// The reader still serves.
	r2 := s.Reader(t)
	expect(t, r2, 221, "HEAD %s", ids[0])
	r2.ReadDotLines()
	expect(t, r2, 205, "QUIT")
}

// checkCachedHead: got has the rewritten Xref:/Path: lines and every other header of the
// spool's copy exactly once, and nothing else.
func checkCachedHead(t *testing.T, name string, got, spoolHead []string, wantXref, wantPath string) {
	t.Helper()
	want := map[string]int{wantXref: 1, wantPath: 1}
	for _, h := range spoolHead {
		if !strings.HasPrefix(h, "Xref:") && !strings.HasPrefix(h, "Path:") {
			want[h]++
		}
	}
	have := map[string]int{}
	for _, l := range got {
		have[l]++
	}
	for l, n := range want {
		if have[l] != n {
			t.Errorf("%s: %d× %q in the cached head, want %d; head %q", name, have[l], l, n, got)
		}
	}
	for l := range have {
		if _, ok := want[l]; !ok {
			t.Errorf("%s: unexpected line %q in the cached head %q", name, l, got)
		}
	}
}

func countNonEmptyFiles(dir string) int {
	n := 0
	filepath.WalkDir(dir, func(_ string, d fs.DirEntry, err error) error {
		if err == nil && d.Type().IsRegular() {
			if fi, err := d.Info(); err == nil && fi.Size() > 0 {
				n++
			}
		}
		return nil
	})
	return n
}

package e2e

// A reader-cache hit was served after the groups check only: the age limit (readerdef
// maxagepolicy, narrowed by the reader's own a<N>) is judged by the spool server, which saw
// only cache misses. Once a reader without that limit had fetched an article into the cache, a
// reader with the limit got it from the cache although the spool server refuses it to them.
// With a limit in force dreaderd now asks the spool server, as on a cache miss.

import (
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/go-while/nntp-testkit/client"
	"github.com/go-while/nntp-testkit/fakeserver"
)

// TestReaderCacheKeepsAgePolicy: readercache on; dreaderd's spool is a real diablo (which applies
// the age) holding one article, which is fed to dreaderd too (so it has a number in e2e.test),
// and fetched at least 2 s after it was stored. Readerdef "short" (maxagepolicy 1, from
// 127.0.0.1) is refused the article (430) by message-id with ARTICLE, HEAD, BODY and STAT, and
// its answers to ARTICLE 1 and BODY 1 after GROUP are noted. Readerdef "long" (maxagepolicy
// 3600, from 127.0.0.3) then fetches it (220), which writes the cache file. A new "short"
// connection gets the same answers as before: 430 by message-id, the noted ones by number.
// "long" is refused with a1 (narrower than its policy) and still gets the article without it.
// a0 never replaces a policy: on the cached article "short" is still refused (430) with a0, "long"
// still gets it, and a reader without a policy (readerdef "none", from 127.0.0.4) is served.
// The spool server's 220 for the article that is cached already must not touch the cache file: no
// *.tmp file appears, and the file keeps its inode and its modification time (comparing the
// article bytes would not show a rewrite with the same bytes).
// Fails before the fix: the second "short" connection got 220, 221, 222 and 223 by message-id
// and the article by number from the cache, and "long" got 220 with a1.
func TestReaderCacheKeepsAgePolicy(t *testing.T) {
	feeds := DefaultNewsfeeds + "label reader\n    inhost 127.0.0.1\n    addgroup *\nend\n"
	feeder := NewSite(t, SiteOptions{Newsfeeds: feeds})
	id := msgID(t)
	art := newArticle(id, "e2e.test", "cached and aged", "body of "+id)
	ihave := func(p *client.Conn) {
		t.Helper()
		p.ExpectCmd(335, "IHAVE %s", id)
		p.MustSend(client.Stuff(art))
		p.ExpectStatus(235, "IHAVE "+id)
		p.ExpectCmd(205, "QUIT")
	}
	ihave(feeder.Peer(t))
	access := `vserverdef DEFAULT
    hostname ` + PathHost + `
end
readerdef short
    read yes
    maxagepolicy 1
end
readerdef long
    read yes
    maxagepolicy 3600
end
readerdef none
    read yes
end
readerdef feed
    feed yes
end
access 127.0.0.1 short
access 127.0.0.3 long
access 127.0.0.4 none
access ` + PeerIP + ` feed
`
	s := NewReaderSite(t, ReaderOptions{SpoolAddr: feeder.Addr, Access: access,
		Config: map[string]string{"readercache": "on"}})
	ihave(s.dialFrom(t, PeerIP))
	// the feeder's history keeps the arrival in minutes: two seconds make the article older
	// than a1 in any case
	time.Sleep(2 * time.Second)

	// do sends cmd and returns its code, reading a multi-line answer
	do := func(c *client.Conn, cmd string) (int, string) {
		t.Helper()
		code, line := c.Cmd("%s", cmd)
		if code == 220 || code == 221 || code == 222 {
			c.ReadDotLines()
		}
		return code, line
	}
	byID := []string{"ARTICLE " + id, "HEAD " + id, "BODY " + id, "STAT " + id}
	byNumber := []string{"ARTICLE 1", "BODY 1"}
	shortAnswers := func(who string) []int {
		t.Helper()
		c := s.Reader(t)
		defer c.Close()
		for _, cmd := range byID {
			if code, line := do(c, cmd); code != 430 {
				t.Errorf("%s: %s: %d %q, want 430", who, cmd, code, line)
			}
		}
		if code, line := c.Cmd("GROUP e2e.test"); code != 211 {
			t.Fatalf("%s: GROUP e2e.test: %d %q", who, code, line)
		}
		var got []int
		for _, cmd := range byNumber {
			code, line := do(c, cmd)
			t.Logf("%s: %s: %d %q", who, cmd, code, line)
			got = append(got, code)
		}
		return got
	}

	before := shortAnswers("maxagepolicy 1, not cached")
	for i, code := range before {
		if code == 220 || code == 222 {
			t.Fatalf("maxagepolicy 1, not cached: %s answered %d; the spool server did not refuse it", byNumber[i], code)
		}
	}

	long := s.dialFrom(t, "127.0.0.3")
	defer long.Close()
	if code, line := do(long, "ARTICLE "+id); code != 220 {
		t.Fatalf("maxagepolicy 3600: ARTICLE %s: %d %q, want 220", id, code, line)
	}
	// The cache file was written at least 1 s before the state below is taken, and the fetches that
	// follow (their last one is checked after another second) come later still: a rewrite would
	// stamp it with a later modification time even on a filesystem that counts whole seconds.
	time.Sleep(1 * time.Second)
	if n := cacheFiles(t, s); n != 1 {
		t.Fatalf("%d cache files after the fetch, want 1: the article was not cached", n)
	}
	cached := cacheDirState(t, s)
	if len(cached.tmp) != 0 {
		t.Fatalf("*.tmp files after the fetch: %q", cached.tmp)
	}

	after := shortAnswers("maxagepolicy 1, cached")
	for i := range byNumber {
		if after[i] != before[i] {
			t.Errorf("maxagepolicy 1: %s: %d with the article cached, %d before", byNumber[i], after[i], before[i])
		}
	}

	if code, line := do(long, "ARTICLE "+id+" a1"); code != 430 {
		t.Errorf("maxagepolicy 3600, cached: ARTICLE %s a1: %d %q, want 430", id, code, line)
	}
	if code, line := do(long, "ARTICLE "+id); code != 220 {
		t.Errorf("maxagepolicy 3600, cached: ARTICLE %s: %d %q, want 220", id, code, line)
	}

	// a0 is no age limit and never replaces a policy: the answers are those without it
	if code, line := do(long, "ARTICLE "+id+" a0"); code != 220 {
		t.Errorf("maxagepolicy 3600, cached: ARTICLE %s a0: %d %q, want 220", id, code, line)
	}
	short := s.Reader(t)
	defer short.Close()
	if code, line := do(short, "ARTICLE "+id+" a0"); code != 430 {
		t.Errorf("maxagepolicy 1, cached: ARTICLE %s a0: %d %q, want 430 (as without a0)", id, code, line)
	}
	if code, line := do(short, "HEAD "+id+" a0"); code != 430 {
		t.Errorf("maxagepolicy 1, cached: HEAD %s a0: %d %q, want 430 (as without a0)", id, code, line)
	}
	none := s.dialFrom(t, "127.0.0.4")
	defer none.Close()
	if code, line := do(none, "ARTICLE "+id+" a0"); code != 220 {
		t.Errorf("no policy, cached: ARTICLE %s a0: %d %q, want 220", id, code, line)
	}

	// The spool server answered 220 to "long" (twice) for the article that is cached already: the
	// cache file is the one it was (CreateCache leaves an existing file alone); the second just
	// given lets a late write or rename show.
	time.Sleep(1 * time.Second)
	cacheUntouched(t, s, cached, "after the spool server's 220 for the cached article")
}

// cacheState is what the reader site's cache directory holds: the committed, non-empty files
// (path to FileInfo; the empty files that mark an uncacheable article are left out) and the
// paths of all *.tmp files, which are the cache files being written.
type cacheState struct {
	files map[string]os.FileInfo
	tmp   []string
}

// cacheDirState reads the reader site's cache directory.
func cacheDirState(t *testing.T, s *Site) cacheState {
	t.Helper()
	st := cacheState{files: map[string]os.FileInfo{}}
	err := filepath.WalkDir(filepath.Join(s.Dir, "spool", "cache"), func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.Type().IsRegular() {
			return nil
		}
		if strings.HasSuffix(path, ".tmp") {
			st.tmp = append(st.tmp, path)
		} else if fi, err := d.Info(); err == nil && fi.Size() > 0 {
			st.files[path] = fi
		}
		return nil
	})
	if err != nil {
		t.Fatalf("cache directory: %v", err)
	}
	return st
}

// cacheFiles counts the committed, non-empty files in the reader site's cache directory (not
// the *.tmp files; cacheDirState lists those).
func cacheFiles(t *testing.T, s *Site) int {
	t.Helper()
	return len(cacheDirState(t, s).files)
}

// cacheUntouched checks that the cache directory still holds the files of before, each with the
// same inode, size and modification time, no other committed file, and no *.tmp file.
func cacheUntouched(t *testing.T, s *Site, before cacheState, when string) {
	t.Helper()
	after := cacheDirState(t, s)
	if len(after.tmp) != 0 {
		t.Errorf("%s: *.tmp files in the cache directory: %q", when, after.tmp)
	}
	for p, b := range before.files {
		a, ok := after.files[p]
		switch {
		case !ok:
			t.Errorf("%s: cache file %s is gone", when, p)
		case !os.SameFile(a, b):
			t.Errorf("%s: cache file %s was replaced (another inode)", when, p)
		case !a.ModTime().Equal(b.ModTime()):
			t.Errorf("%s: cache file %s was written: modification time %v, was %v", when, p, a.ModTime(), b.ModTime())
		case a.Size() != b.Size():
			t.Errorf("%s: cache file %s has size %d, was %d", when, p, a.Size(), b.Size())
		}
	}
	for p := range after.files {
		if _, ok := before.files[p]; !ok {
			t.Errorf("%s: new cache file %s", when, p)
		}
	}
}

// TestReaderCacheAgeLimitAsksSpool: readercache on; the spool is a fake server that stands in
// for diablo's age rule: it answers ARTICLE <id> with the article, and refuses it (430) when
// the request carries a<N> below 3600. Readerdef "none" (no policy, from 127.0.0.1) fetches the
// article (one spool request, no age argument) and is then served from the cache (no further
// request). Readerdef "short" (maxagepolicy 60, from 127.0.0.3) is asked for at the spool server
// with a60 and gets 430; "none" with its own a30 likewise; readerdef "long" (maxagepolicy 3600,
// from 127.0.0.4) is asked for with a3600 and gets the article. "none" is still served from the
// cache afterwards, with the same article. a0 is no age limit and replaces no policy: "short"
// with a0 is asked for with a60 and gets 430 as without it, "none" with a0 is served from the
// cache (no spool request). The 220 for "long" does not touch the cache file: no *.tmp file
// appears, and it keeps its inode and modification time. Fails before the fix: "short" and
// "none ... a30" were served 220 from the cache and the spool server saw no request with an age
// argument.
func TestReaderCacheAgeLimitAsksSpool(t *testing.T) {
	id := msgID(t)
	art := newArticle(id, "e2e.test", "age limit and cache", "body of "+id)
	handler := func(c *fakeserver.Conn) {
		for {
			line, err := c.ReadLine()
			if err != nil {
				return
			}
			f := strings.Fields(line)
			switch {
			case len(f) >= 2 && strings.EqualFold(f[0], "ARTICLE") && f[1] == id:
				if len(f) >= 3 && strings.HasPrefix(f[2], "a") {
					if n, err := strconv.Atoi(f[2][1:]); err == nil && n < 3600 {
						c.WriteLines("430 Article prohibited")
						continue
					}
				}
				reply := append([]string{"220 0 " + id + " article follows"}, art...)
				c.WriteLines(append(reply, ".")...)
			case len(f) > 0 && strings.EqualFold(f[0], "QUIT"):
				c.WriteLines("205 bye")
				return
			default:
				c.WriteLines("430 no")
			}
		}
	}
	sp := fakeserver.New(t, fakeserver.Options{Handler: handler})
	access := `vserverdef DEFAULT
    hostname ` + PathHost + `
end
readerdef none
    read yes
end
readerdef short
    read yes
    maxagepolicy 60
end
readerdef long
    read yes
    maxagepolicy 3600
end
access 127.0.0.1 none
access 127.0.0.3 short
access 127.0.0.4 long
`
	s := NewReaderSite(t, ReaderOptions{Spool: sp, Access: access, Config: map[string]string{"readercache": "on"}})
	// asked returns the ARTICLE lines the spool server got
	asked := func() []string {
		var a []string
		for _, l := range sp.Lines() {
			if f := strings.Fields(l); len(f) > 0 && strings.EqualFold(f[0], "ARTICLE") {
				a = append(a, l)
			}
		}
		return a
	}
	fetch := func(c *client.Conn, who, opt string, want int) []string {
		t.Helper()
		cmd := "ARTICLE " + id
		if opt != "" {
			cmd += " " + opt
		}
		code, line := c.Cmd("%s", cmd)
		var lines []string
		if code == 220 {
			lines = c.ReadDotLines()
		}
		if code != want {
			t.Errorf("%s: %s: %d %q, want %d", who, cmd, code, line, want)
		}
		return lines
	}
	// last checks that the spool server got n ARTICLE requests, the last one with the age
	// argument want ("" = none)
	last := func(who string, n int, want string) {
		t.Helper()
		a := asked()
		if len(a) != n {
			t.Errorf("%s: %d spool requests %q, want %d", who, len(a), a, n)
			return
		}
		f := strings.Fields(a[n-1])
		got := ""
		if len(f) >= 3 {
			got = f[2]
		}
		if got != want {
			t.Errorf("%s: spool request %q, want the age argument %q", who, a[n-1], want)
		}
	}

	none := s.Reader(t)
	defer none.Close()
	first := fetch(none, "no policy", "", 220)
	last("no policy, first fetch", 1, "")
	time.Sleep(1 * time.Second)
	fetch(none, "no policy, cached", "", 220)
	if n := len(asked()); n != 1 {
		t.Fatalf("no policy, cached: %d spool requests, want 1: the article was not served from the cache", n)
	}
	// The cache file was written at least 1 s before this state is taken, and the spool answers
	// below come later (the last is checked after another second): a rewrite would stamp it with
	// a later modification time even on a filesystem that counts whole seconds.
	cached := cacheDirState(t, s)
	if n := len(cached.files); n != 1 || len(cached.tmp) != 0 {
		t.Fatalf("cache directory: %d committed files, *.tmp %q; want 1 and none", n, cached.tmp)
	}

	short := s.dialFrom(t, "127.0.0.3")
	defer short.Close()
	fetch(short, "maxagepolicy 60, cached", "", 430)
	last("maxagepolicy 60, cached", 2, "a60")

	fetch(none, "no policy, cached", "a30", 430)
	last("no policy with a30, cached", 3, "a30")

	long := s.dialFrom(t, "127.0.0.4")
	defer long.Close()
	fetch(long, "maxagepolicy 3600, cached", "", 220)
	last("maxagepolicy 3600, cached", 4, "a3600")
	time.Sleep(1 * time.Second)
	cacheUntouched(t, s, cached, "after the spool server's 220 for the cached article")

	again := fetch(none, "no policy, cached after the others", "", 220)
	if n := len(asked()); n != 4 {
		t.Errorf("no policy, cached after the others: %d spool requests, want 4: not served from the cache", n)
	}
	if strings.Join(again, "\n") != strings.Join(first, "\n") {
		t.Errorf("no policy: the article from the cache changed:\nfirst: %q\nlater: %q", first, again)
	}

	// a0 replaces no policy: "short" is asked for with a60 and refused as without it; "none" has
	// none to keep and is served from the cache, with no spool request
	fetch(short, "maxagepolicy 60 with a0, cached", "a0", 430)
	last("maxagepolicy 60 with a0, cached", 5, "a60")
	fetch(none, "no policy with a0, cached", "a0", 220)
	if n := len(asked()); n != 5 {
		t.Errorf("no policy with a0, cached: %d spool requests, want 5: not served from the cache", n)
	}
}

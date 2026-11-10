package e2e

import (
	"encoding/binary"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unsafe"

	testkit "github.com/go-while/nntp-testkit"
)

// TestDeepRevScoringCacheRatioFraction (L5-06): with "cache=scoring(r,c)" in dserver.hosts
// an article is cached on its first read once the group's cache hits divided by its reads
// from the spool exceed c (dreaderd/cache.c, CreateCache()). The division was an integer
// one, so the quotient was 0 until the hits reached the reads: a ratio between 0 and 1,
// like 0.3, only took effect at 1.0, and 1.0 at 2.0.
//
// The scoreboard entry of the group is set to 10 reads and 5 hits (0.5, with the next read
// 5/11). With scoring(-1,0.3) the next article read must be cached at once.
func TestDeepRevScoringCacheRatioFraction(t *testing.T) {
	if unsafe.Sizeof(uintptr(0)) != 8 || !littleEndian() {
		t.Skip("writes a 64-bit little-endian cache.hits")
	}
	st, spool := spoolStore(t)
	s := NewReaderSite(t, ReaderOptions{Spool: spool, NoStart: true, Config: map[string]string{"readercache": "on"}})
	s.WriteFile(t, "dserver.hosts", "127.0.0.1\ts1p"+strings.TrimPrefix(spool.Addr(), "127.0.0.1:")+"\tcache=scoring(-1,0.3)\n")
	startReader(t, s, "[127.0.0.1]:"+strings.TrimPrefix(s.Addr, "127.0.0.1:"))
	feedReader(t, s, st, "e2e.test",
		newArticle(msgID(t), "e2e.test", "first", "body one"),
		newArticle(msgID(t), "e2e.test", "second", "body two"))

	cached := func() []string {
		var files []string
		filepath.WalkDir(filepath.Join(s.Dir, "spool", "cache"), func(p string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || strings.HasSuffix(p, ".tmp") {
				return nil
			}
			if fi, err := d.Info(); err == nil && fi.Size() > 0 {
				files = append(files, p)
			}
			return nil
		})
		return files
	}

	// The first read makes the group's entry: 1 read, no hit, ratio 0, nothing cached.
	if ok, d := readArticleOnce(t, s, "e2e.test", 1); !ok {
		t.Fatalf("first read: %s", d)
	}
	path := filepath.Join(s.Dir, "cache.hits")
	var es []cacheHitsEntry
	testkit.WaitFor(t, 10*time.Second, "a scoreboard entry", func() bool {
		es = cacheHitsEntries(t, path)
		return len(es) > 0
	})
	if len(es) != 1 {
		t.Fatalf("scoreboard entries: %+v, want one", es)
	}
	if c := cached(); len(c) != 0 {
		t.Fatalf("cached after the first read of the group (ratio 0): %v", c)
	}

	f, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	var b [8]byte
	binary.LittleEndian.PutUint32(b[0:], 10) // che_ReadArt
	binary.LittleEndian.PutUint32(b[4:], 5)  // che_Hits
	if _, err := f.WriteAt(b[:], es[0].off+cheReadArt); err != nil {
		t.Fatal(err)
	}
	f.Close()

	if ok, d := readArticleOnce(t, s, "e2e.test", 2); !ok {
		t.Fatalf("second read: %s", d)
	}
	limit := time.Now().Add(5 * time.Second)
	for len(cached()) == 0 && time.Now().Before(limit) {
		time.Sleep(100 * time.Millisecond)
	}
	if len(cached()) == 0 {
		t.Errorf("no cache file after reading an article of a group with 5 hits on 11 reads and scoring(-1,0.3): the ratio 0.45 was taken as 0 (scoreboard %+v)",
			cacheHitsEntries(t, path))
	}
}

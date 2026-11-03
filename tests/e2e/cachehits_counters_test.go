package e2e

// The cache.hits scoreboard (cache=scoring in dserver.hosts) is a file that every
// dreaderd reader process maps MAP_SHARED and updates for each article read: che_ReadArt
// for a read that missed the article cache, che_Hits for a cache hit.  dexpirescoring
// decays the same counters from cron.
//
//	ZeroReadCount  CreateCache() divided che_Hits by che_ReadArt.  The read just counted
//	               keeps che_ReadArt at 1 or more only while nothing else touches it; a
//	               lost update between two readers, the "partly cached" ReadArt--, or a
//	               dexpirescoring decay to 0 leaves it at 0 (or below) and the reader
//	               process died of SIGFPE, dropping every session it served.  The test
//	               sets che_ReadArt to -1 in the shared file, as a lost update can leave
//	               it, so that the next miss counts it up to exactly 0.  (An integer
//	               divide by zero traps on x86 but not on arm64, where main passes.)
//	OneEntry       The scoreboard key (struct CacheHash_t: two uint64 and an int) was
//	               compared and copied with memcmp/memcpy over its full size, including
//	               the 4 bytes of padding after the int that SetCacheHash() never set.
//	               What was on the stack decided whether a group's entry was found: on
//	               main the hit path and the miss path keep separate entries for one
//	               group (every time, on x86-64 with gcc), so the Hits/ReadArt ratio of
//	               cache=scoring stayed 0, and dexpirescoring may find neither.
//	Concurrent     Several reader processes read the same cached article at once.  Each
//	               read must count exactly once (che_Hits++ was a plain read-modify-write
//	               of the shared map) and the group must keep a single entry.
//
// The test reads cache.hits directly: 64-bit little-endian layout only.

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
	"unsafe"
)

// lib/defs.h CacheHitEntry on LP64: che_hash (a CacheHitHead, 32 bytes, holding the
// CacheHash_t h1, h2, iter), then che_Next, che_ReadArt, che_Hits, che_LastHi, che_NewArt,
// padded to 56 bytes.
const (
	cheSize    = 56
	cheReadArt = 36
	cheHits    = 40
)

type cacheHitsEntry struct {
	off          int64
	h1, h2       uint64
	iter         int32
	readArt      int32
	hits, lastHi uint32
}

// cacheHitsEntries returns the entries allocated in the scoreboard at path.
func cacheHitsEntries(tb testing.TB, path string) []cacheHitsEntry {
	tb.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		tb.Fatal(err)
	}
	h := readCacheHitsHead(tb, path)
	if h.magic != 0xD1C2B3A4 || h.newEntry > uint32(len(b)) {
		tb.Fatalf("cache.hits: bad header %+v (file %d bytes)", h, len(b))
	}
	le := binary.LittleEndian
	var out []cacheHitsEntry
	for off := 32 + int64(h.hashSize)*4; off+cheSize <= int64(h.newEntry); off += cheSize {
		e := b[off:]
		out = append(out, cacheHitsEntry{
			off:     off,
			h1:      le.Uint64(e[0:]),
			h2:      le.Uint64(e[8:]),
			iter:    int32(le.Uint32(e[16:])),
			readArt: int32(le.Uint32(e[cheReadArt:])),
			hits:    le.Uint32(e[cheHits:]),
			lastHi:  le.Uint32(e[44:]),
		})
	}
	return out
}

// scoringReaderSite starts dreaderd with the spool server in scoreboard cache mode.
// scoring(-1,-1) caches every article on its first read, so the second read of an
// article is a cache hit.
func scoringReaderSite(t *testing.T, conf map[string]string) (*Site, func(group string, arts ...[]string)) {
	t.Helper()
	if unsafe.Sizeof(uintptr(0)) != 8 || !littleEndian() {
		t.Skip("reads a 64-bit little-endian cache.hits")
	}
	st, spool := spoolStore(t)
	c := map[string]string{"readercache": "on"}
	for k, v := range conf {
		c[k] = v
	}
	s := NewReaderSite(t, ReaderOptions{Spool: spool, NoStart: true, Config: c})
	s.WriteFile(t, "dserver.hosts", "127.0.0.1\ts1p"+strings.TrimPrefix(spool.Addr(), "127.0.0.1:")+"\tcache=scoring(-1,-1)\n")
	startReader(t, s, "[127.0.0.1]:"+strings.TrimPrefix(s.Addr, "127.0.0.1:"))
	return s, func(group string, arts ...[]string) { feedReader(t, s, st, group, arts...) }
}

// readArticleOnce reads one article on a new session; ok is false when the session
// ended before the article did (the reader process died).
func readArticleOnce(t *testing.T, s *Site, group string, n int) (ok bool, detail string) {
	t.Helper()
	nc, err := net.DialTimeout("tcp", s.Addr, 10*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer nc.Close()
	nc.SetDeadline(time.Now().Add(20 * time.Second))
	r := bufio.NewReader(nc)
	if _, err := r.ReadString('\n'); err != nil {
		return false, "greeting: " + err.Error()
	}
	fmt.Fprintf(nc, "GROUP %s\r\nARTICLE %d\r\n", group, n)
	l, err := r.ReadString('\n')
	if err != nil || !strings.HasPrefix(l, "211 ") {
		return false, fmt.Sprintf("GROUP: %q %v", l, err)
	}
	l, err = r.ReadString('\n')
	if err != nil || !strings.HasPrefix(l, "220 ") {
		return false, fmt.Sprintf("ARTICLE: %q %v", l, err)
	}
	for {
		l, err = r.ReadString('\n')
		if err != nil {
			return false, "article text: " + err.Error()
		}
		if l == ".\r\n" {
			return true, ""
		}
	}
}

func TestReaderCacheHitsZeroReadCount(t *testing.T) {
	s, feed := scoringReaderSite(t, nil)
	path := filepath.Join(s.Dir, "cache.hits")
	id1, id2 := msgID(t), msgID(t)
	feed("e2e.test", newArticle(id1, "e2e.test", "first", "body one"))

	if ok, d := readArticleOnce(t, s, "e2e.test", 1); !ok {
		t.Fatalf("first read: %s", d)
	}
	var es []cacheHitsEntry
	waitFor(t, 10*time.Second, "a scoreboard entry", func() bool {
		es = cacheHitsEntries(t, path)
		return len(es) > 0
	})
	if len(es) != 1 || es[0].readArt != 1 {
		t.Fatalf("after one miss: entries %+v, want one with ReadArt 1", es)
	}

	// ReadArt -1 in the shared map: the next miss in this group counts it to 0.
	f, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	var b [4]byte
	binary.LittleEndian.PutUint32(b[:], 0xFFFFFFFF)
	if _, err := f.WriteAt(b[:], es[0].off+cheReadArt); err != nil {
		t.Fatal(err)
	}
	f.Close()

	feed("e2e.test", newArticle(id2, "e2e.test", "second", "body two"))
	if ok, d := readArticleOnce(t, s, "e2e.test", 2); !ok {
		t.Fatalf("a miss with che_ReadArt counted up to 0 ended the session (the reader process died): %s", d)
	}
	// The reader still serves.
	if ok, d := readArticleOnce(t, s, "e2e.test", 1); !ok {
		t.Fatalf("read after the zero count: %s", d)
	}
}

func TestReaderCacheHitsOneEntry(t *testing.T) {
	s, feed := scoringReaderSite(t, nil)
	path := filepath.Join(s.Dir, "cache.hits")
	var ids []string
	for i := 0; i < 3; i++ {
		id := msgID(t)
		ids = append(ids, id)
		feed("e2e.test", newArticle(id, "e2e.test", "one entry", "body"))
	}
	// Each article: a miss (CreateCache counts ReadArt), then two hits
	// (DumpArticleFromCache counts Hits).
	for n := range ids {
		for i := 0; i < 3; i++ {
			if ok, d := readArticleOnce(t, s, "e2e.test", n+1); !ok {
				t.Fatalf("read %d: %s", n+1, d)
			}
		}
	}
	es := cacheHitsEntries(t, path)
	if len(es) != 1 {
		t.Fatalf("scoreboard entries for one group: %d, want 1: %+v", len(es), es)
	}
	if es[0].readArt != 3 || es[0].hits != 6 {
		t.Errorf("entry ReadArt=%d Hits=%d, want 3 and 6", es[0].readArt, es[0].hits)
	}
	// dexpirescoring finds the entry for the group in dactive.kp.
	out := s.Run(t, "dexpirescoring", "-v")
	if !strings.Contains(out, "e2e.test") {
		t.Errorf("dexpirescoring -v did not find the e2e.test entry:\n%s", out)
	}
}

// TestReaderCacheHitsConcurrent reads one cached article from many sessions spread
// over four reader processes, pipelining ARTICLE commands so that the processes update
// che_Hits at the same time.  Every read must be counted, in a single entry.
//
// The lost-update part is statistical: che_Hits++ on the shared map is a load and a
// store a few nanoseconds apart.  On main (12 cores) every run lost between 5 and 34
// of the 9600 counts; one core, or a slow machine, may lose none.  The entry count
// fails on main every time (see OneEntry).  E2E_CACHEHITS_READS raises the reads
// per session.
func TestReaderCacheHitsConcurrent(t *testing.T) {
	s, feed := scoringReaderSite(t, map[string]string{"readerforks": "4", "readerthreads": "8"})
	path := filepath.Join(s.Dir, "cache.hits")
	id := msgID(t)
	feed("e2e.test", newArticle(id, "e2e.test", "concurrent", "body"))
	// The miss that caches the article.
	if ok, d := readArticleOnce(t, s, "e2e.test", 1); !ok {
		t.Fatalf("first read: %s", d)
	}

	const sessions = 24
	perSession := 400
	if v := os.Getenv("E2E_CACHEHITS_READS"); v != "" {
		fmt.Sscan(v, &perSession)
	}
	const batch = 50
	var wg sync.WaitGroup
	errs := make(chan error, sessions)
	start := make(chan struct{})
	for i := 0; i < sessions; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			nc, err := net.DialTimeout("tcp", s.Addr, 10*time.Second)
			if err != nil {
				errs <- err
				return
			}
			defer nc.Close()
			nc.SetDeadline(time.Now().Add(5 * time.Minute))
			r := bufio.NewReader(nc)
			if _, err := r.ReadString('\n'); err != nil {
				errs <- err
				return
			}
			fmt.Fprintf(nc, "GROUP e2e.test\r\n")
			if l, err := r.ReadString('\n'); err != nil || !strings.HasPrefix(l, "211 ") {
				errs <- fmt.Errorf("GROUP: %q %v", l, err)
				return
			}
			<-start
			cmd := strings.Repeat("ARTICLE 1\r\n", batch)
			for done := 0; done < perSession; done += batch {
				if _, err := io.WriteString(nc, cmd); err != nil {
					errs <- err
					return
				}
				for j := 0; j < batch; j++ {
					l, err := r.ReadString('\n')
					if err != nil || !strings.HasPrefix(l, "220 ") {
						errs <- fmt.Errorf("ARTICLE: %q %v", l, err)
						return
					}
					for {
						l, err = r.ReadString('\n')
						if err != nil {
							errs <- err
							return
						}
						if l == ".\r\n" {
							break
						}
					}
				}
			}
		}()
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatalf("session: %v", err)
	}

	es := cacheHitsEntries(t, path)
	if len(es) != 1 {
		t.Errorf("scoreboard entries for one group: %d, want 1: %+v", len(es), es)
	}
	// Lost updates, whichever entries the reads went to.
	var hits uint32
	var readArt int32
	for _, e := range es {
		hits += e.hits
		readArt += e.readArt
	}
	want := uint32(sessions * perSession)
	if hits != want || readArt != 1 {
		t.Errorf("Hits=%d ReadArt=%d, want %d and 1 (%d cache hits lost)",
			hits, readArt, want, int64(want)-int64(hits))
	}
}

// TestDexpirescoringLastEntry: dexpirescoring skipped an entry that ends exactly at
// the end of the scoreboard (its walk stopped at offsets >= end - entry size).  The
// test takes the key dreaderd wrote for e2e.test and rebuilds cache.hits with that
// entry last: with the default 4096 buckets, 291 other entries put it at 32712 and
// the end at 32768.  dexpirescoring must find it and decay its counters.
func TestDexpirescoringLastEntry(t *testing.T) {
	s, feed := scoringReaderSite(t, nil)
	path := filepath.Join(s.Dir, "cache.hits")
	feed("e2e.test", newArticle(msgID(t), "e2e.test", "last entry", "body"))
	if ok, d := readArticleOnce(t, s, "e2e.test", 1); !ok {
		t.Fatalf("first read: %s", d)
	}
	var es []cacheHitsEntry
	waitFor(t, 10*time.Second, "a scoreboard entry", func() bool {
		es = cacheHitsEntries(t, path)
		return len(es) > 0
	})
	key := es[0]
	hashSize := readCacheHitsHead(t, path).hashSize
	if hashSize == 0 {
		t.Fatal("cache.hits: hash size 0")
	}

	const n = 292
	start := 32 + int64(hashSize)*4
	end := start + n*cheSize
	b := make([]byte, end)
	le := binary.LittleEndian
	le.PutUint32(b[0:], 0xD1C2B3A4)
	le.PutUint32(b[4:], 1)
	le.PutUint32(b[8:], hashSize)
	le.PutUint32(b[12:], uint32(end))
	le.PutUint32(b[16:], uint32(end))
	le.PutUint64(b[24:], uint64(time.Now().Unix()-36000))
	// add appends an entry to its bucket's chain, as CreateCacheHitEntry does.
	add := func(off int64, h1, h2 uint64, iter int32) {
		e := b[off:]
		le.PutUint64(e[0:], h1)
		le.PutUint64(e[8:], h2)
		le.PutUint32(e[16:], uint32(iter))
		link := 32 + int64((h1^h2)%uint64(hashSize))*4
		for le.Uint32(b[link:]) != 0 {
			link = int64(le.Uint32(b[link:])) + 32 // che_Next
		}
		le.PutUint32(b[link:], uint32(off))
	}
	for i := int64(0); i < n-1; i++ {
		h1, h2 := uint64(0xE2E0000000000000)|uint64(i), uint64(i)
		if h1 == key.h1 && h2 == key.h2 {
			h1++
		}
		add(start+i*cheSize, h1, h2, 0)
	}
	last := end - cheSize
	add(last, key.h1, key.h2, key.iter)
	le.PutUint32(b[last+cheReadArt:], 400)
	le.PutUint32(b[last+cheHits:], 800)
	le.PutUint32(b[last+44:], key.lastHi)
	// A new file: the reader keeps its mapping of the old one.
	if err := os.WriteFile(path+".new", b, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(path+".new", path); err != nil {
		t.Fatal(err)
	}

	out := s.Run(t, "dexpirescoring", "-v", "-H", "36000")
	if !strings.Contains(out, "e2e.test") {
		t.Errorf("dexpirescoring -v did not find the entry at %d (end %d):\n%s", last, end, out)
	}
	es = cacheHitsEntries(t, path)
	if len(es) != n || es[n-1].off != last {
		t.Fatalf("scoreboard after dexpirescoring: %d entries, want %d", len(es), n)
	}
	// Halved (a second more of delay rounds down by one).
	if e := es[n-1]; e.readArt < 195 || e.readArt > 200 || e.hits < 395 || e.hits > 400 {
		t.Errorf("last entry ReadArt=%d Hits=%d, want about 200 and 400 (not decayed)", e.readArt, e.hits)
	}
}

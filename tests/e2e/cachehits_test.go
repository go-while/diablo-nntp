package e2e

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unsafe"
)

// cacheHitsHead is lib/defs.h's CacheHitHead on a 64-bit system: five uint32, padding,
// a 64-bit time_t (32 bytes).
type cacheHitsHead struct {
	magic, version, hashSize, newEntry, end uint32
	size                                    int64
}

func readCacheHitsHead(tb testing.TB, path string) cacheHitsHead {
	tb.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		tb.Fatal(err)
	}
	if len(b) < 32 {
		return cacheHitsHead{size: int64(len(b))}
	}
	le := binary.LittleEndian
	return cacheHitsHead{le.Uint32(b[0:]), le.Uint32(b[4:]), le.Uint32(b[8:]), le.Uint32(b[12:]), le.Uint32(b[16:]), int64(len(b))}
}

// TestReaderCacheHitsRepaired: a damaged scoreboard (cache.hits with a good magic and version
// but a header that claims more than the file holds, as an interrupted growth used to leave)
// is replaced by a new one when dreaderd opens it. Before, every reader logged "invalid or
// truncated cache.hits" and ran without the scoreboard until an admin deleted the file.
// 64-bit little-endian only (the test writes the raw header).
func TestReaderCacheHitsRepaired(t *testing.T) {
	if unsafe.Sizeof(uintptr(0)) != 8 || !littleEndian() {
		t.Skip("writes a 64-bit little-endian cache.hits header")
	}
	st, spool := spoolStore(t)
	s := NewReaderSite(t, ReaderOptions{
		Spool:   spool,
		NoStart: true,
		Config: map[string]string{
			"readercache":     "on",
			"path_generallog": "%s/general.log",
		},
	})
	// The spool server with scoreboard caching; replaces the generated dserver.hosts.
	s.WriteFile(t, "dserver.hosts", "127.0.0.1\ts1p"+strings.TrimPrefix(spool.Addr(), "127.0.0.1:")+"\tcache=scoring(1,1)\n")

	// A damaged scoreboard: 20 KiB of file, a header claiming 1 MiB.
	const hashSize = 4096
	b := make([]byte, 20480)
	le := binary.LittleEndian
	le.PutUint32(b[0:], 0xD1C2B3A4)
	le.PutUint32(b[4:], 1)
	le.PutUint32(b[8:], hashSize)
	le.PutUint32(b[12:], 32+hashSize*4)
	le.PutUint32(b[16:], 1<<20)
	path := filepath.Join(s.Dir, "cache.hits")
	if err := os.WriteFile(path, b, 0o644); err != nil {
		t.Fatal(err)
	}

	startReader(t, s, "[127.0.0.1]:"+strings.TrimPrefix(s.Addr, "127.0.0.1:"))
	id := msgID(t)
	feedReader(t, s, st, "e2e.test", newArticle(id, "e2e.test", "cache.hits", "body"))
	c := s.Reader(t)
	expect(t, c, 211, "GROUP e2e.test")
	expect(t, c, 220, "ARTICLE %s", id)
	c.ReadDotLines()
	expect(t, c, 205, "QUIT")

	var h cacheHitsHead
	waitFor(t, 10*time.Second, "a usable cache.hits", func() bool {
		h = readCacheHitsHead(t, path)
		return h.magic == 0xD1C2B3A4 && h.end <= uint32(h.size) && h.newEntry <= h.end && h.hashSize > 0
	})
	if h.end >= 1<<20 {
		t.Errorf("cache.hits header still claims %d bytes (file %d)", h.end, h.size)
	}
	logb, _ := os.ReadFile(filepath.Join(s.Dir, "log", "general.log"))
	if !strings.Contains(string(logb), "cache.hits damaged") {
		t.Errorf("no log line about replacing the damaged cache.hits")
	}
}

func littleEndian() bool {
	x := uint16(1)
	return *(*byte)(unsafe.Pointer(&x)) == 1
}

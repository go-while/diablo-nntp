package e2e

import (
	"crypto/md5"
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type hashfeedMode struct {
	name     string
	selector string
	offset   int
	legacy   bool
}

var hashfeedModes = []hashfeedMode{
	{name: "md5-offset-0", selector: "%d/2"},
	{name: "md5-offset-4", selector: "%d/2:4", offset: 4},
	{name: "md5-offset-8", selector: "%d/2:8", offset: 8},
	{name: "md5-offset-12", selector: "%d/2:12", offset: 12},
	{name: "legacy-quickhash", selector: "@%d/2", legacy: true},
}

// bucket mirrors Diablo's documented hashfeed calculations: the legacy mode
// sums Message-ID bytes; MD5 modes use the selected four-byte digest window as
// a big-endian integer. Both map the value to buckets numbered 1..modulus.
func (m hashfeedMode) bucket(id string, modulus uint32) uint32 {
	if m.legacy {
		var sum uint32
		for _, b := range []byte(id) {
			sum += uint32(b)
		}
		return sum%modulus + 1
	}
	digest := md5.Sum([]byte(id))
	start := 12 - m.offset
	return binary.BigEndian.Uint32(digest[start:start+4])%modulus + 1
}

func (m hashfeedMode) messageIDs(modulus uint32) []string {
	ids := make([]string, modulus)
	for i := 0; i < 10000; i++ {
		id := fmt.Sprintf("<hashfeed-%d@e2e.test>", i)
		bucket := m.bucket(id, modulus) - 1
		if ids[bucket] == "" {
			ids[bucket] = id
		}
		complete := true
		for _, found := range ids {
			if found == "" {
				complete = false
				break
			}
		}
		if complete {
			break
		}
	}
	return ids
}

func TestDnewsfeedsHashfeedFiltersByMessageID(t *testing.T) {
	for _, mode := range hashfeedModes {
		t.Run(mode.name, func(t *testing.T) {
			ids := mode.messageIDs(2)
			for i, id := range ids {
				if id == "" {
					t.Fatalf("could not find a Message-ID in hashfeed bucket %d", i+1)
				}
			}
			feeds := GlobalLabel + `label peer
    inhost ` + PeerIP + `
    alias e2e.peer
    addgroup *
end
label ` + OutLabel + `
    hostname 127.0.0.1
    addgroup *
    hashfeed ` + fmt.Sprintf(mode.selector, 1) + `
end
`
			s := NewSite(t, SiteOptions{Newsfeeds: feeds})
			p := s.Peer(t)
			for _, id := range ids {
				expect(t, p, 335, "IHAVE %s", id)
				send(t, p, wire(newArticle(id, "e2e.test", "hashfeed", "body")))
				status(t, p, 235, "IHAVE "+id)
			}
			expect(t, p, 205, "QUIT")

			q := s.waitQueue(t, OutLabel, 1)
			if !strings.Contains(q[0], ids[0]) {
				t.Errorf("dqueue/%s = %q, want bucket 1 Message-ID %s", OutLabel, q, ids[0])
			}
			if strings.Contains(strings.Join(q, "\n"), ids[1]) {
				t.Errorf("dqueue/%s contains bucket 2 Message-ID %s: %q", OutLabel, ids[1], q)
			}
		})
	}
}

func TestDspoolHashfeedSelectsSpoolByMessageID(t *testing.T) {
	for _, mode := range hashfeedModes {
		t.Run(mode.name, func(t *testing.T) {
			ids := mode.messageIDs(2)
			for i, id := range ids {
				if id == "" {
					t.Fatalf("could not find a Message-ID in hashfeed bucket %d", i+1)
				}
			}
			spoolCtl := `spool 01
    path @home@/spool/news/a
    minfree 1m
end
spool 02
    path @home@/spool/news/b
    minfree 1m
end
metaspool bucket1
    hashfeed ` + fmt.Sprintf(mode.selector, 1) + `
    spool 01
end
metaspool bucket2
    hashfeed ` + fmt.Sprintf(mode.selector, 2) + `
    spool 02
end
expire * bucket1
expire * bucket2
`
			spoolNames := []string{"a", "b"}
			for bucket, id := range ids {
				t.Run(fmt.Sprintf("bucket-%d", bucket+1), func(t *testing.T) {
					s := NewSite(t, SiteOptions{
						SpoolCtl: spoolCtl,
						Setup: func(tb testing.TB, s *Site) {
							for _, dir := range []string{"spool/news/a", "spool/news/b"} {
								if err := os.MkdirAll(filepath.Join(s.Dir, dir), 0o755); err != nil {
									tb.Fatal(err)
								}
							}
						},
					})
					p := s.Peer(t)
					expect(t, p, 335, "IHAVE %s", id)
					send(t, p, wire(newArticle(id, "e2e.test", "hashfeed spool", "body")))
					status(t, p, 235, "IHAVE "+id)
					expect(t, p, 205, "QUIT")

					want := filepath.Join(s.Dir, "spool", "news", spoolNames[bucket])
					other := filepath.Join(s.Dir, "spool", "news", spoolNames[1-bucket])
					glob := filepath.Join(want, "D.*", "*")
					waitFor(t, 5*time.Second, "article in hashfeed-selected spool", func() bool {
						files, _ := filepath.Glob(glob)
						return len(files) > 0
					})
					files, err := filepath.Glob(glob)
					if err != nil || len(files) == 0 {
						t.Fatalf("Message-ID %s was not stored under %s (glob error %v)", id, want, err)
					}
					if files, _ := filepath.Glob(filepath.Join(other, "D.*", "*")); len(files) != 0 {
						t.Errorf("Message-ID %s was also stored on the other spool: %v", id, files)
					}
				})
			}
		})
	}
}

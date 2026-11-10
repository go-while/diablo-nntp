package e2e

import (
	"bytes"
	"encoding/binary"
	"os"
	"testing"
)

// TestDeepRevDamagedSpoolHeader (L1-07): every stored article starts with a small header
// (SpoolArtHdr) that holds its own length and the length of the article's head. For an
// article that is not compressed diablo did not check them before SendArticle() stepped over
// that many bytes of the mapped file: a damaged header made BODY answer 222 with data from
// the wrong place or none at all, HEAD run past the head, or the session crash. Such an
// article must be answered 430, as a compressed one with a damaged header already is.
func TestDeepRevDamagedSpoolHeader(t *testing.T) {
	for _, c := range []struct {
		name   string
		damage func(hdr []byte)
	}{
		// uint32 ArtHdrLen at offset 8 (after five bytes and padding)
		{"arthdrlen", func(h []byte) { binary.NativeEndian.PutUint32(h[8:], 0x7fffffff) }},
		// uint8 HeadLen at offset 3
		{"headlen", func(h []byte) { h[3] = 200 }},
	} {
		t.Run(c.name, func(t *testing.T) {
			s := NewSite(t, SiteOptions{})
			id := s.Inject(t, "e2e.test", 1)[0]
			p := s.Peer(t)
			p.ExpectCmd(222, "BODY %s", id)
			p.ReadDotLines()
			p.ExpectCmd(205, "QUIT")

			file := spoolFileOf(t, s, id)
			b, err := os.ReadFile(file)
			if err != nil {
				t.Fatal(err)
			}
			// The first article of the file: its header is at offset 0.
			if len(b) < 24 || !bytes.Equal(b[:2], []byte{0xff, 0x99}) {
				t.Fatalf("%s does not start with a spool article header: % x", file, b[:min(len(b), 24)])
			}
			c.damage(b)
			if err := os.WriteFile(file, b, 0o644); err != nil {
				t.Fatal(err)
			}

			for _, cmd := range []string{"BODY", "HEAD", "ARTICLE"} {
				p := s.Peer(t)
				code, line, err := p.TryCmd("%s %s", cmd, id)
				if err != nil || code != 430 {
					t.Errorf("%s of an article with a damaged spool header (%s): %q, %v; want 430", cmd, c.name, line, err)
				}
				p.Close()
			}
		})
	}
}

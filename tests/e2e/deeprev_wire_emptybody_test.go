package e2e

import (
	"bytes"
	"encoding/binary"
	"os"
	"testing"
)

// TestDeepRevWireFormatEmptyBody: with "wireformat on" an article is stored as it came in,
// and for BODY diablo steps over the stored head and its separator and sends the rest as it
// is, terminator included. A spool header whose head length takes the whole entry (or all
// but a byte or two) left nothing to send: the client got "222" and then nothing, and
// waited. The reply must be complete: 430, or 222 with its terminator.
func TestDeepRevWireFormatEmptyBody(t *testing.T) {
	for _, short := range []int{0, 1, 2} {
		s := NewSite(t, SiteOptions{Config: map[string]string{"wireformat": "on"}})
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
		if len(b) < 24 || !bytes.Equal(b[:2], []byte{0xff, 0x99}) {
			t.Fatalf("%s does not start with a spool article header: % x", file, b[:min(len(b), 24)])
		}
		// uint32 ArtHdrLen at offset 8, uint32 ArtLen at offset 12: the head now takes
		// the whole stored article but "short" bytes.
		artLen := binary.NativeEndian.Uint32(b[12:])
		binary.NativeEndian.PutUint32(b[8:], artLen-uint32(short))
		if err := os.WriteFile(file, b, 0o644); err != nil {
			t.Fatal(err)
		}

		p = s.Peer(t)
		code, line, err := p.TryCmd("BODY %s", id)
		switch {
		case err != nil:
			t.Errorf("head length = article length - %d: BODY: %v", short, err)
		case code == 222:
			if _, err := p.TryReadDotLines(); err != nil {
				t.Errorf("head length = article length - %d: BODY answered %q and then no complete reply: %v", short, line, err)
			}
		case code != 430:
			t.Errorf("head length = article length - %d: BODY: %q, want 430 or a complete 222 reply", short, line)
		}
		p.Close()
	}
}

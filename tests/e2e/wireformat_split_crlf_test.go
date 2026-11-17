package e2e

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/go-while/nntp-testkit/client"
)

// TestWireFormatCRLFSplitAcrossBuffer (deep review r2 M1): the feeder reads an article in
// pieces of at most the input buffer's size (bu_DataMax: one page less the Buffer header and
// an int). A line of exactly bu_DataMax-1 octets before its CRLF comes as one piece that
// ends in the CR and a second piece that is the LF alone. Text mode puts the CR back and
// reads it with the LF; wire mode does not, so that line does not count as CRLF-ended and
// the "." after it is taken as data: the article never ends. The buffer size is computed
// for the two Buffer layouts (with and without zlib, 64-bit) from the page size; the
// control is the same article in text mode.
func TestWireFormatCRLFSplitAcrossBuffer(t *testing.T) {
	page := os.Getpagesize()
	var lens []int
	for _, hdr := range []int{64, 56} { // offsetof(Buffer, bu_Buf) with and without zlib
		lens = append(lens, page-hdr-4-1)
	}
	for _, mode := range []string{"off", "on"} {
		t.Run("wireformat-"+mode, func(t *testing.T) {
			s := NewSite(t, SiteOptions{Config: map[string]string{"wireformat": mode}})
			for _, n := range lens {
				p := s.Peer(t)
				id := msgID(t)
				p.ExpectCmd(335, "IHAVE %s", id)
				p.MustSend(client.Stuff(newArticle(id, "e2e.test", "split crlf", "first", strings.Repeat("x", n))))
				p.Deadline = 10 * time.Second
				code, line, err := p.ReadStatus()
				if code != 235 {
					t.Errorf("article whose last line has %d octets (page %d): %d %q (%v), want 235 within 10 s",
						n, page, code, line, err)
				}
				p.Close()
			}
		})
	}
}

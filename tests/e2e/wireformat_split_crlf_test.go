package e2e

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	testkit "github.com/go-while/nntp-testkit"
	"github.com/go-while/nntp-testkit/client"
)

// splitCRLFLengths returns line lengths whose CRLF falls on the boundary of two pieces of the
// feeder's input buffer. The feeder reads an article in pieces of at most bu_DataMax octets
// (one page less the Buffer header and an int); a line longer than that comes in pieces of
// exactly bu_DataMax octets from its start. A line of k x bu_DataMax - 1 octets therefore has
// its CR as the last octet of a full piece and its LF alone in the next one. The header size
// differs with and without zlib (64-bit), so both layouts are covered, for k = 1 and 2.
func splitCRLFLengths() []int {
	page := os.Getpagesize()
	var lens []int
	for _, hdr := range []int{64, 56} { // offsetof(Buffer, bu_Buf) with and without zlib
		d := page - hdr - 4
		lens = append(lens, d-1, 2*d-1)
	}
	return lens
}

// lastBodyLine returns the last line of an article read with ReadDotLines.
func lastBodyLine(lines []string) string {
	if len(lines) == 0 {
		return ""
	}
	return lines[len(lines)-1]
}

// TestWireFormatCRLFSplitAcrossBuffer (deep review r2 M1, D-229): with "wireformat on" a
// line whose CR is the last octet of a full input-buffer piece and whose LF is the next
// piece did not count as CRLF-ended (text mode puts the CR back and reads it with the LF;
// wire mode did not), so the "." after it was taken as data: an IHAVE article never ended
// (no reply), and over TAKETHIS the next command and article became part of this one. Each
// article must be accepted on its own and stored with its bytes unchanged; the control is
// the same in text mode.
func TestWireFormatCRLFSplitAcrossBuffer(t *testing.T) {
	lens := splitCRLFLengths()
	for _, mode := range []string{"off", "on"} {
		t.Run("wireformat-"+mode, func(t *testing.T) {
			s := NewSite(t, SiteOptions{Config: map[string]string{"wireformat": mode}})

			// IHAVE: the long line is the last line of the body
			for _, n := range lens {
				p := s.Peer(t)
				id := msgID(t)
				long := strings.Repeat("x", n)
				p.ExpectCmd(335, "IHAVE %s", id)
				p.MustSend(client.Stuff(newArticle(id, "e2e.test", "split crlf", "first", long)))
				p.Deadline = 10 * time.Second
				code, line, err := p.ReadStatus()
				if code != 235 {
					t.Errorf("IHAVE article whose last line has %d octets (page %d): %d %q (%v), want 235 within 10 s",
						n, os.Getpagesize(), code, line, err)
					p.Close()
					continue
				}
				p.ExpectCmd(220, "ARTICLE %s", id)
				if got := lastBodyLine(p.ReadDotLines()); got != long {
					t.Errorf("stored article: last line has %d octets, want the %d sent", len(got), n)
				}
				p.ExpectCmd(205, "QUIT")
				p.Close()
			}

			// TAKETHIS, pipelined: an article whose last line splits, then a second one whose
			// first body line splits. Each gets its own 239 and is stored on its own.
			for _, n := range lens {
				p := s.Peer(t)
				p.ExpectCmd(203, "MODE STREAM")
				long := strings.Repeat("y", n)
				id1, id2 := msgID(t), msgID(t)
				raw1 := client.Stuff(newArticle(id1, "e2e.test", "split crlf 1", "first", long))
				raw2 := client.Stuff(newArticle(id2, "e2e.test", "split crlf 2", long, "after"))
				p.MustSend(fmt.Sprintf("TAKETHIS %s\r\n%sTAKETHIS %s\r\n%s", id1, raw1, id2, raw2))
				p.Deadline = 10 * time.Second
				ok := true
				for _, id := range []string{id1, id2} {
					code, line, err := p.ReadStatus()
					if code != 239 || !strings.Contains(line, id) {
						t.Errorf("TAKETHIS with a line of %d octets: %d %q (%v), want 239 %s within 10 s",
							n, code, line, err, id)
						ok = false
						break
					}
				}
				if !ok {
					p.Close()
					continue
				}
				p.ExpectCmd(220, "ARTICLE %s", id1)
				if got := lastBodyLine(p.ReadDotLines()); got != long {
					t.Errorf("stored article 1: last line has %d octets, want the %d sent", len(got), n)
				}
				p.ExpectCmd(220, "ARTICLE %s", id2)
				lines := p.ReadDotLines()
				if len(lines) < 2 || lines[len(lines)-2] != long || lines[len(lines)-1] != "after" {
					t.Errorf("stored article 2 ends %q, want the %d-octet line and \"after\"",
						lines[max(0, len(lines)-2):], n)
				}
				p.ExpectCmd(205, "QUIT")
				p.Close()
				if mode == "on" {
					// every byte of both articles counted once (the session line counts the
					// bytes received, the terminators included)
					want := fmt.Sprintf("recbytes=%d ", len(raw1)+len(raw2))
					testkit.WaitFor(t, 10*time.Second, "session line "+want, func() bool {
						return strings.Contains(feederLogs(s), want)
					})
				}
			}
		})
	}
}

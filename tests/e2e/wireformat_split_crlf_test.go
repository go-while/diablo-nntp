package e2e

import (
	"bytes"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	testkit "github.com/go-while/nntp-testkit"
	"github.com/go-while/nntp-testkit/client"
)

// splitDataMaxes returns the candidate sizes of the feeder's input-buffer pieces. The feeder
// reads an article in pieces of at most bu_DataMax octets (lib/buffer.c bopen: one page, as
// feederbuffersize is 1, less offsetof(Buffer, bu_Buf) and an int); a line longer than that
// comes in pieces of exactly bu_DataMax octets from its start. offsetof(Buffer, bu_Buf) is 44
// on a 32-bit build without zlib and 64 on a 64-bit build with zlib, and lies between them with
// 4- or 8-byte pointers (six ints, three or four pointers, two more ints). The candidates cover
// 40..72 in steps of 4, so for these layouts one of them is the real bu_DataMax and the
// cases built from it put a CR at the end of a full piece: the test cannot pass without
// hitting the boundary (on main it fails there, see the README).
func splitDataMaxes() []int {
	page := os.Getpagesize()
	var ds []int
	for h := 40; h <= 72; h += 4 {
		ds = append(ds, page-h-4)
	}
	return ds
}

// splitRaiseMaxLine lets c read lines of up to n octets. nntp-testkit from v0.14.1 caps one
// line at Conn.MaxLine (64 KiB by default), which the lines of splitCases exceed on a system
// with 64 KiB pages (aarch64, ppc64le); older kits have neither the field nor a cap. The field
// is set by name so that the test builds with both.
func splitRaiseMaxLine(c *client.Conn, n int) {
	if f := reflect.ValueOf(c).Elem().FieldByName("MaxLine"); f.IsValid() && f.CanSet() && f.Kind() == reflect.Int {
		f.SetInt(int64(n))
	}
}

// splitCase is the last body line of an article, with its line end, as sent and as stored.
type splitCase struct {
	name string
	body string
}

// splitCases returns, for a piece size d, lines whose CR (or CR CR) falls on the boundary of
// two pieces: a line of d - 1 octets and one of 2d - 1 (the CR ends a full piece, the LF is
// the next), CR CR LF with both CRs or only the first at the end of the piece, and a lone CR
// that ends the piece with more text after it.
func splitCases(d int) []splitCase {
	return []splitCase{
		{"crlf-1", strings.Repeat("x", d-1) + "\r\n"},
		{"crlf-2", strings.Repeat("x", 2*d-1) + "\r\n"},
		{"crcrlf-both", strings.Repeat("x", d-2) + "\r\r\n"},
		{"crcrlf-first", strings.Repeat("x", d-1) + "\r\r\n"},
		{"lone-cr", strings.Repeat("x", d-1) + "\rmore\r\n"},
	}
}

// splitArticleRaw returns the article id with body "first" and then body, as sent (the
// lines of body are not dot-stuffed: none starts with "."), terminator included.
func splitArticleRaw(id, extraHeader, body string) string {
	head, _ := splitArticle(newArticle(id, "e2e.test", "split crlf"))
	if extraHeader != "" {
		head = append(head[:1], append([]string{extraHeader}, head[1:]...)...)
	}
	return strings.Join(head, "\r\n") + "\r\n\r\nfirst\r\n" + body + ".\r\n"
}

// TestWireFormatCRLFSplitAcrossBuffer (D-229): with "wireformat on" a
// line whose CR is the last octet of a full input-buffer piece and whose LF is the next
// piece did not count as CRLF-ended (text mode puts the CR back and reads it with the LF;
// wire mode did not), so the "." after it was taken as data: an IHAVE article did not end
// (no reply), and over TAKETHIS the next command and article were read as part of this one. Each
// article must be accepted on its own and its body must come back as sent, for every
// candidate piece size (splitDataMaxes) and every case of splitCases, plus a header line
// that splits the same way; the control is the same in text mode.
func TestWireFormatCRLFSplitAcrossBuffer(t *testing.T) {
	ds := splitDataMaxes()
	t.Logf("page %d, candidate piece sizes %v", os.Getpagesize(), ds)
	for _, mode := range []string{"off", "on"} {
		t.Run("wireformat-"+mode, func(t *testing.T) {
			s := NewSite(t, SiteOptions{Config: map[string]string{"wireformat": mode}})

			// IHAVE, one connection: an article that never ends leaves it unusable
			p := s.Peer(t)
			p.Deadline = 10 * time.Second
			p.Strict = false // stored lines with a CR before the CRLF are read raw
			splitRaiseMaxLine(p, 2*os.Getpagesize()+4096)
		ihave:
			for _, d := range ds {
				for _, c := range splitCases(d) {
					id := msgID(t)
					p.ExpectCmd(335, "IHAVE %s", id)
					p.MustSend(splitArticleRaw(id, "", c.body))
					if code, line, err := p.ReadStatus(); code != 235 {
						t.Errorf("IHAVE %s (piece size %d, %d octets): %d %q (%v), want 235 within 10 s",
							c.name, d, len(c.body), code, line, err)
						break ihave
					}
					p.ExpectCmd(220, "ARTICLE %s", id)
					if raw := p.RawUntilDot(); !bytes.Contains(raw, []byte("\r\nfirst\r\n"+c.body+".\r\n")) {
						t.Errorf("IHAVE %s (piece size %d): stored article does not end with the %d octets sent",
							c.name, d, len(c.body))
					}
				}
			}
			p.Close()

			// TAKETHIS, pipelined per piece size: the cases, then an article whose header line
			// splits. Each gets its own 239, and ARTICLE of each holds its own body as sent.
			p = s.Peer(t)
			p.Deadline = 10 * time.Second
			p.Strict = false
			splitRaiseMaxLine(p, 2*os.Getpagesize()+4096)
			p.ExpectCmd(203, "MODE STREAM")
			total, ok := 0, true
		takethis:
			for _, d := range ds {
				var ids, bodies []string
				var b strings.Builder
				for _, c := range splitCases(d) {
					id := msgID(t)
					raw := splitArticleRaw(id, "", c.body)
					fmt.Fprintf(&b, "TAKETHIS %s\r\n%s", id, raw)
					ids, bodies, total = append(ids, id), append(bodies, c.body), total+len(raw)
				}
				hdr := "X-Split: " + strings.Repeat("z", d-1-len("X-Split: "))
				id := msgID(t)
				raw := splitArticleRaw(id, hdr, "after\r\n")
				fmt.Fprintf(&b, "TAKETHIS %s\r\n%s", id, raw)
				ids, bodies, total = append(ids, id), append(bodies, "after\r\n"), total+len(raw)
				p.MustSend(b.String())
				for _, id := range ids {
					if code, line, err := p.ReadStatus(); code != 239 || !strings.Contains(line, id) {
						t.Errorf("TAKETHIS (piece size %d): %d %q (%v), want 239 %s within 10 s", d, code, line, err, id)
						ok = false
						break takethis
					}
				}
				for i, id := range ids {
					p.ExpectCmd(220, "ARTICLE %s", id)
					raw := p.RawUntilDot()
					if !bytes.Contains(raw, []byte("\r\nfirst\r\n"+bodies[i]+".\r\n")) {
						t.Errorf("TAKETHIS (piece size %d) article %d: stored body is not the one sent", d, i+1)
					}
					if i == len(ids)-1 && !bytes.Contains(raw, []byte("\r\n"+hdr+"\r\n")) {
						t.Errorf("TAKETHIS (piece size %d): the %d-octet header line is not stored as sent", d, len(hdr))
					}
				}
			}
			if !ok {
				p.Close()
				return
			}
			p.ExpectCmd(205, "QUIT")
			p.Close()
			if mode == "on" {
				// every byte of the articles counted once (the session line counts the bytes
				// received, the terminators included; text mode counts a put-back CR twice)
				want := fmt.Sprintf("recbytes=%d ", total)
				testkit.WaitFor(t, 10*time.Second, "session line "+want, func() bool {
					return strings.Contains(feederLogs(s), want)
				})
			}
		})
	}
}

// TestWireFormatBareCRAtBufferBoundary (D-229): with
// "rejectartswithbarecr on", a bare CR (one not followed by LF) is refused, also when it is
// the last octet of a full input-buffer piece: with the CR read again with the next piece it
// is checked against the octet after it. On main a bare CR there was missed in wire mode
// (the article was accepted); text mode already put it back. Every candidate piece size
// (splitDataMaxes) must get 437, in both modes.
func TestWireFormatBareCRAtBufferBoundary(t *testing.T) {
	ds := splitDataMaxes()
	for _, mode := range []string{"off", "on"} {
		t.Run("wireformat-"+mode, func(t *testing.T) {
			s := NewSite(t, SiteOptions{Config: map[string]string{"wireformat": mode, "rejectartswithbarecr": "on"}})
			p := s.Peer(t)
			p.Deadline = 10 * time.Second
			for _, d := range ds {
				id := msgID(t)
				p.ExpectCmd(335, "IHAVE %s", id)
				p.MustSend(splitArticleRaw(id, "", strings.Repeat("x", d-1)+"\rmore\r\n"))
				if code, line, err := p.ReadStatus(); code != 437 {
					t.Errorf("IHAVE with a bare CR at octet %d of a line (piece size %d): %d %q (%v), want 437",
						d, d, code, line, err)
				}
			}
			p.ExpectCmd(205, "QUIT")
		})
	}
}

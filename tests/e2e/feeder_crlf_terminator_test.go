package e2e

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	testkit "github.com/go-while/nntp-testkit"
)

// TestFeederBareLFDotIsData: only CRLF "." CRLF ends an article (RFC 3977 3.1.1). A body
// line that is just "." and ends with a bare LF is data, as is a "." after a bare LF. On
// main the feeder read ".\n" as the end of the article, kept only the part before it, and
// took the rest of the body for commands: one 500 per line, and a peer such as innxmit
// closes on the first 500.
//
// The article mixes CRLF and bare-LF lines the way a Spotnet post did on an archive feed.
// The article must be accepted whole (recbytes in the session line is every byte sent,
// the terminator included) and the next command must be answered normally.
func TestFeederBareLFDotIsData(t *testing.T) {
	for _, wire := range []string{"off", "on"} {
		t.Run("wireformat "+wire, func(t *testing.T) {
			id := msgID(t)
			art := newArticle(id, "e2e.test", "bare LF dot", "x")
			head, _ := splitArticle(art)
			raw := strings.Join(head, "\r\n") + "\r\n\r\n" +
				"quote.\r\n" +
				"text ended by a bare LF.\n" +
				".\n" + // data, not the end of the article
				".dotted after a bare LF\n" + // no unstuffing after a bare LF
				"still the body\r\n" +
				"bare again\n" +
				".\r\n" + // CRLF after a bare LF: a "." that starts after a bare LF is data
				"..stuffed after CRLF\r\n" + // unstuffed: one dot is cut
				".\r\n" // the end

			s := NewSite(t, SiteOptions{Config: map[string]string{"wireformat": wire}})
			p := s.Peer(t)
			p.ExpectCmd(203, "MODE STREAM")

			if err := p.Send(fmt.Sprintf("TAKETHIS %s\r\n%s", id, raw)); err != nil {
				t.Fatal(err)
			}
			p.ExpectStatus(239, "TAKETHIS with a bare-LF . line")

			// The next command is answered as a command, not read as the rest of the article.
			id2 := msgID(t)
			p.ExpectCmd(238, "CHECK %s", id2)

			// With wireformat off the stored body is the article's whole body, unstuffed
			// by one dot only after CRLF. (With it on the bytes are stored as they came and
			// sent back the same way, so a "." LF line would end the reading side's view.)
			if wire == "off" {
				p.Strict = false
				p.ExpectCmd(220, "ARTICLE %s", id)
				lines := p.ReadDotLines()
				i := slices.Index(lines, "")
				want := []string{"quote.", "text ended by a bare LF.", ".", ".dotted after a bare LF",
					"still the body", "bare again", ".", ".stuffed after CRLF"}
				if i < 0 || !slices.Equal(lines[i+1:], want) {
					t.Errorf("stored body %q, want %q", lines, want)
				}
			}

			p.ExpectCmd(205, "QUIT")

			// Every byte of the article was taken (the session line counts the bytes received,
			// the terminator included): no part of it was left for the command loop.
			want := fmt.Sprintf("recbytes=%d ", len(raw))
			testkit.WaitFor(t, 10*time.Second, "session line "+want, func() bool {
				return strings.Contains(feederLogs(s), want)
			})
		})
	}
}

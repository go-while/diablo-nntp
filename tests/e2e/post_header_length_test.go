package e2e

import (
	"strings"
	"testing"

	"github.com/go-while/nntp-testkit/client"
)

// TestReaderPostHeaderLength checks physical lines, not unfolded field length
// (RFC 5536 2.2). RFC mode rejects 999 octets, accepts 998 and long folded
// fields; explicit legacy mode keeps accepting long header lines. An oversized
// article is drained and the same connection remains usable after its 441.
func TestReaderPostHeaderLength(t *testing.T) {
	const prefix = "X-Long: "
	for _, mode := range []struct {
		name, access string
		strict       bool
	}{
		{"default", DefaultReaderAccess, true},
		{"rfc5537 no", plainAccess("    rfc5537 no\n"), false},
		{"legacytrace yes", plainAccess("    legacytrace yes\n"), false},
	} {
		t.Run(mode.name, func(t *testing.T) {
			d := newDatePoster(t, mode.access, nil)
			c := d.s.Reader(t)
			defer c.Close()
			for _, tc := range []struct {
				name  string
				lines []string
				long  bool
			}{
				{"998 first", []string{prefix + strings.Repeat("x", 998-len(prefix))}, false},
				{"999 first", []string{prefix + strings.Repeat("x", 999-len(prefix))}, true},
				{"998 continuation", []string{prefix + "x", " " + strings.Repeat("x", 997)}, false},
				{"999 continuation", []string{prefix + "x", "\t" + strings.Repeat("x", 998)}, true},
				{"long folded field", []string{prefix + strings.Repeat("x", 650), " " + strings.Repeat("x", 650)}, false},
				{"short after refusal", []string{prefix + "short"}, false},
			} {
				t.Run(tc.name, func(t *testing.T) {
					c.ExpectCmd(340, "POST")
					art := []string{"From: Poster <poster@example.invalid>", "Newsgroups: e2e.test",
						"Subject: header length", "Message-ID: " + msgID(t)}
					art = append(art, tc.lines...)
					art = append(art, "", "body", "last body line")
					c.MustSend(client.Stuff(art))
					want := 240
					if mode.strict && tc.long {
						want = 441
					}
					code, line, err := c.ReadStatus()
					if err != nil {
						t.Fatalf("POST: %v", err)
					}
					if code != want {
						t.Errorf("POST: %q, want %d", line, want)
					}
					if code == 441 && !strings.Contains(line, "998") {
						t.Errorf("refusal %q does not name the header limit", line)
					}
					c.ExpectCmd(111, "DATE")
				})
			}
		})
	}
}

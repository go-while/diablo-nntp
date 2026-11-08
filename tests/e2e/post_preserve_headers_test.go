package e2e

import (
	"slices"
	"strings"
	"testing"
	"time"

	testkit "github.com/go-while/nntp-testkit"
	"github.com/go-while/nntp-testkit/client"
)

// TestReaderPostHeaderPreservation checks the original header order, spelling,
// whitespace and folding at the outgoing peer (RFC 5537 3.5 step 6). It also
// runs in the isolated build with the optional legacy cleanup macros enabled.
func TestReaderPostHeaderPreservation(t *testing.T) {
	for _, addresses := range []bool{true, false} {
		name := "without-addresses"
		if addresses {
			name = "with-addresses"
		}
		t.Run(name, func(t *testing.T) {
			d := newDatePoster(t, DefaultReaderAccess, nil)
			want := []string{
				"From: Poster <poster@example.invalid>", "X-First: first",
				"To: recipient@example.invalid", "Newsgroups:\te2e.test , e2e.other \t",
				"Subject: unchanged headers", "X-Folded: first part", "\tsecond part",
				"Cc: copy@example.invalid", "Distribution: local", "lInEs:\t0002 \t",
				"Date: " + rfcDate(0), "Message-ID: " + msgID(t), "X-Last: last",
			}
			if !addresses {
				want = slices.DeleteFunc(want, func(h string) bool { return strings.HasPrefix(h, "To:") || strings.HasPrefix(h, "Cc:") })
			}
			c := d.s.Reader(t)
			defer c.Close()
			c.ExpectCmd(340, "POST")
			c.MustSend(client.Stuff(slices.Concat(want, []string{"", "first", "last"})))
			c.ExpectStatus(240, "POST preserving valid headers")
			testkit.WaitFor(t, 10*time.Second, "preserved POST at outgoing peer", func() bool {
				d.mu.Lock()
				defer d.mu.Unlock()
				return len(d.arts) == 1
			})
			d.mu.Lock()
			head, _ := splitArticle(slices.Clone(d.arts[0]))
			d.mu.Unlock()
			var got []string
			skip := false
			for _, h := range head {
				if strings.HasPrefix(h, " ") || strings.HasPrefix(h, "\t") {
					if !skip {
						got = append(got, h)
					}
					continue
				}
				k, _, _ := strings.Cut(h, ":")
				skip = strings.EqualFold(k, "Path") || strings.EqualFold(k, "Injection-Info") || strings.EqualFold(k, "Injection-Date")
				if skip {
					continue
				}
				got = append(got, h)
			}
			if !slices.Equal(got, want) {
				t.Errorf("outgoing headers:\n%q\nwant unchanged:\n%q", got, want)
			}
		})
	}
}

func TestReaderPostLinesSyntax(t *testing.T) {
	for _, mode := range []struct {
		name, vs string
		strict   bool
	}{
		{"default", "", true}, {"rfc5537 no", "    rfc5537 no\n", false},
		{"legacytrace yes", "    legacytrace yes\n", false},
	} {
		t.Run(mode.name, func(t *testing.T) {
			d := newDatePoster(t, plainAccess(mode.vs), nil)
			for _, tc := range []struct {
				name    string
				headers []string
				bad     bool
			}{
				{"zero", []string{"Lines: 0"}, false},
				{"leading zeros", []string{"lInEs:\t00099 \t"}, false},
				{"large decimal", []string{"Lines: " + strings.Repeat("9", 100)}, false},
				{"empty", []string{"Lines: "}, true},
				{"text", []string{"Lines: unknown"}, true},
				{"negative", []string{"Lines: -1"}, true},
				{"plus", []string{"Lines: +1"}, true},
				{"two numbers", []string{"Lines: 1 2"}, true},
				{"comment", []string{"Lines: 1 (one)"}, true},
				{"folded", []string{"Lines: 1", " 2"}, true},
				{"duplicate", []string{"Lines: 1", "lInEs: 1"}, true},
			} {
				t.Run(tc.name, func(t *testing.T) {
					code, line, head := d.post(t, tc.headers...)
					want := 240
					if mode.strict && tc.bad {
						want = 441
					}
					if code != want {
						t.Fatalf("POST: %q, want %d", line, want)
					}
					if code == 441 && !strings.Contains(line, "Lines") {
						t.Errorf("wrong refusal: %q", line)
					}
					if code == 240 {
						var got []string
						for _, h := range head {
							k, _, _ := strings.Cut(h, ":")
							if strings.EqualFold(k, "Lines") {
								got = append(got, h)
							}
						}
						var expected []string
						for _, h := range tc.headers {
							if !strings.HasPrefix(h, " ") && !strings.HasPrefix(h, "\t") {
								expected = append(expected, h)
							}
						}
						if !slices.Equal(got, expected) {
							t.Errorf("Lines %q, want preserved %q", got, expected)
						}
					}
				})
			}
		})
	}
}

func TestReaderPostEmptyDistribution(t *testing.T) {
	for _, mode := range []struct {
		name, vs string
		strict   bool
	}{
		{"default", "", true}, {"rfc5537 no", "    rfc5537 no\n", false},
		{"legacytrace yes", "    legacytrace yes\n", false},
	} {
		t.Run(mode.name, func(t *testing.T) {
			d := newDatePoster(t, plainAccess(mode.vs), nil)
			for _, h := range [][]string{{"Distribution: "}, {"Distribution:\t \t"}, {"Distribution: local", "Distribution: "}} {
				code, line, _ := d.post(t, h...)
				want := 240
				if mode.strict {
					want = 441
				}
				if code != want {
					t.Errorf("POST %q: %q, want %d", h, line, want)
				}
				if code == 441 && !strings.Contains(line, "Distribution") {
					t.Errorf("wrong refusal: %q", line)
				}
			}
		})
	}
}

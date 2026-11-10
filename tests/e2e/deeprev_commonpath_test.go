package e2e

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/go-while/nntp-testkit/client"
)

// TestDeepRevCommonPathBareCR (L1-01): with "commonpath" (diablo -c) set, the feeder looks for
// that name in the Path: of every article (lib/newsfeed.c, CommonElmMatches()). The scan stops
// at a CR but only steps over a '!', so a CR inside the Path: value, with at least as many
// bytes behind it as the common name is long, was never passed: the session process looped at
// 100 % CPU for good, answered nothing more, and ignored "dicmd exit" and SIGTERM (the peer
// then reconnects and offers the article again: one more looping process each time).
// rejectartswithbarecr, which would refuse the article first, is off by default.
//
// The article must be answered: accepted (235) by default, refused (437) with
// rejectartswithbarecr on. A folded Path: (CRLF and a blank) has no CR in the value and was
// never affected; it is checked so a fix does not break it.
func TestDeepRevCommonPathBareCR(t *testing.T) {
	const common = "common.e2e.invalid"
	tail := strings.Repeat("c", 2*len(common)) + "!not-for-mail"
	for _, c := range []struct {
		name   string
		option string // rejectartswithbarecr, "" = default (off)
		edit   func(art []string) []string
		want   int
		stored bool // the stored Path: must carry the common name
	}{
		{"cr_in_path", "", func(a []string) []string { a[0] = "Path: e2e.peer!a\r" + tail; return a }, 235, true},
		{"cr_first_in_path", "", func(a []string) []string { a[0] = "Path: \r" + tail; return a }, 235, true},
		{"cr_in_path_rejectartswithbarecr", "on", func(a []string) []string { a[0] = "Path: e2e.peer!a\r" + tail; return a }, 437, false},
		{"folded_path", "", func(a []string) []string {
			a[0] = "Path: e2e.peer!a!"
			return slices.Insert(a, 1, " b!not-for-mail")
		}, 235, true},
	} {
		t.Run(c.name, func(t *testing.T) {
			conf := map[string]string{"commonpath": common}
			if c.option != "" {
				conf["rejectartswithbarecr"] = c.option
			}
			s := NewSite(t, SiteOptions{Config: conf})
			deepRevKillChildren(t, s, "diablo")

			p := s.Peer(t)
			p.Deadline = 10 * time.Second // a looping session never answers
			id := msgID(t)
			art := c.edit(newArticle(id, "e2e.test", "commonpath and a CR in Path", "body"))
			p.ExpectCmd(335, "IHAVE %s", id)
			p.MustSend(client.Stuff(art))
			code, line, err := p.ReadStatus()
			if err != nil {
				t.Fatalf("no reply to the article within %v: %v (line %q); the session process loops in CommonElmMatches()",
					p.Deadline, err, line)
			}
			if code != c.want {
				t.Fatalf("IHAVE %s: got %q, want %d", id, line, c.want)
			}
			if c.stored {
				// The stored header may hold the bare CR: read it without the strict line checks.
				p.Strict = false
				p.ExpectCmd(221, "HEAD %s", id)
				head := p.ReadDotLines()
				i := slices.IndexFunc(head, func(l string) bool { return strings.HasPrefix(l, "Path: ") })
				if i < 0 || !strings.Contains(head[i], common+"!") {
					t.Errorf("the stored Path: does not carry the common path element %q: %q", common, head)
				}
			}
			p.ExpectCmd(205, "QUIT")
		})
	}
}

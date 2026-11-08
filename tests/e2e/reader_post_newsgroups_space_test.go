package e2e

import (
	"strings"
	"sync"
	"testing"

	"github.com/go-while/nntp-testkit/fakeserver"
)

// postGroupsWildAccess lets 127.0.0.1 post to the groups that match e2e.t* only.
const postGroupsWildAccess = `vserverdef DEFAULT
    hostname ` + PathHost + `
end
groupdef tstar
    group e2e.t*
end
readerdef rp
    read yes
    post yes
    groups tstar
end
readerdef feed
    feed yes
end
access 127.0.0.1 rp
access ` + PeerIP + ` feed
`

// TestReaderPostNewsgroupsSpace: RFC 5536 3.1.4 allows white space in the Newsgroups: list only
// around the commas. A POST whose list has white space between two names ("a b") is refused
// with 441 Invalid newsgroup(s) line and not passed on. Before, dreaderd checked such a list as
// one name, which a wildcard groups entry (e2e.t*) matched as a whole. A list without such white
// space is checked by its plain comma list, so white space before a comma is fine (D-206).
func TestReaderPostNewsgroupsSpace(t *testing.T) {
	_, spool := spoolStore(t)
	var mu sync.Mutex
	var sent []string
	post := fakeserver.New(t, fakeserver.Options{Handler: fakeserver.PeerHandler(&fakeserver.PeerOptions{
		OnArticle: func(id string, wire []string) {
			mu.Lock()
			defer mu.Unlock()
			sent = append(sent, id)
		},
	})})
	s := NewReaderSite(t, ReaderOptions{Spool: spool, Post: post, Access: postGroupsWildAccess})

	for _, c := range []struct {
		groups string
		want   int
	}{
		{"e2e.test", 240},
		{"e2e.test e2e.other", 441},  // white space between two names
		{"e2e.test\te2e.other", 441}, // a tab between two names
		{"e2e.other e2e.test", 441},
		{"e2e.test ,e2e.test", 240}, // white space before a comma is allowed
		{"e2e.other", 441},          // not in the groups list: refused as before
	} {
		id := msgID(t)
		c2 := s.Reader(t)
		c2.ExpectCmd(340, "POST")
		c2.MustSend(strings.Join([]string{
			"From: E2E Poster <poster@example.invalid>",
			"Newsgroups: " + c.groups,
			"Subject: groups " + c.groups,
			"Message-ID: " + id,
			"",
			"body",
		}, "\r\n") + "\r\n.\r\n")
		code, line, err := c2.ReadStatus()
		c2.Close()
		if err != nil {
			t.Fatalf("POST with Newsgroups: %q: %v", c.groups, err)
		}
		if code != c.want {
			t.Errorf("POST with Newsgroups: %q: %q, want %d", c.groups, line, c.want)
		}
		if c.want == 441 && strings.ContainsAny(c.groups, " \t") && !strings.HasSuffix(c.groups, " ,e2e.test") &&
			!strings.Contains(line, "Invalid newsgroup") {
			t.Errorf("POST with Newsgroups: %q: %q, want \"Invalid newsgroup(s) line\"", c.groups, line)
		}
	}
	mu.Lock()
	defer mu.Unlock()
	if len(sent) > 2 {
		t.Errorf("%d articles passed on, want at most the 2 accepted ones: %q", len(sent), sent)
	}
}

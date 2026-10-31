package e2e

import (
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-while/nntp-testkit/fakeserver"
)

// TestReaderPostBareLFHeaders: dreaderd checks the headers of a POST line by line. A bare LF
// starts a new line for the server the post is passed to, so it must start one for the
// checks too: a refused header after a bare LF is refused like the same header on its own
// CRLF line (441), and an article sent entirely with bare LF line ends (old clients) is still
// accepted and reaches the outgoing server with each header on its own line. Before the fix
// the checks split headers only on CRLF, so "Subject: x<LF>NNTP-Posting-Host: y" was one
// Subject header to them and two headers to the receiving server; a Control: header with
// "controlpost no" got through the same way.
func TestReaderPostBareLFHeaders(t *testing.T) {
	_, spool := spoolStore(t)
	var mu sync.Mutex
	var arts [][]string
	post := fakeserver.New(t, fakeserver.Options{Handler: fakeserver.PeerHandler(&fakeserver.PeerOptions{
		OnArticle: func(id string, wire []string) {
			mu.Lock()
			defer mu.Unlock()
			arts = append(arts, wire)
		},
	})})
	access := strings.Replace(DefaultReaderAccess, "    post yes\n", "    post yes\n    controlpost no\n", 1)
	s := NewReaderSite(t, ReaderOptions{Spool: spool, Post: post, Access: access})

	head := func(id string) []string {
		return []string{
			"From: E2E Poster <poster@example.invalid>",
			"Newsgroups: e2e.test",
			"Message-ID: " + id,
		}
	}
	postRaw := func(raw string) (int, string) {
		c := s.Reader(t)
		defer c.Close()
		expect(t, c, 340, "POST")
		send(t, c, raw)
		code, line, err := c.ReadStatus()
		if err != nil {
			t.Fatalf("POST: %v", err)
		}
		return code, line
	}
	crlf := func(lines []string) string { return strings.Join(lines, "\r\n") + "\r\n.\r\n" }

	for _, hidden := range []string{
		"NNTP-Posting-Host: forged.example.invalid",
		"Control: newgroup e2e.forged",
	} {
		name := hidden[:strings.Index(hidden, ":")]
		// The header on its own line is refused ...
		lines := append(head(msgID(t)), "Subject: own line", hidden, "", "body")
		if code, line := postRaw(crlf(lines)); code != 441 {
			t.Errorf("%s on its own CRLF line: %q, want 441", name, line)
		}
		// ... and so it must be after a bare LF inside another header line.
		lines = append(head(msgID(t)), "Subject: hidden\n"+hidden, "", "body")
		if code, line := postRaw(crlf(lines)); code != 441 {
			t.Errorf("%s after a bare LF in Subject: %q, want 441", name, line)
		}
	}

	// A post with bare LF line ends throughout is accepted, and each header arrives on its
	// own line.
	id := msgID(t)
	lines := append(head(id), "Subject: all bare LF", "", "body line")
	if code, line := postRaw(strings.Join(lines, "\n") + "\n.\n"); code != 240 {
		t.Fatalf("bare-LF post: %q, want 240", line)
	}
	var got []string
	waitFor(t, 15*time.Second, "the outgoing server to receive the bare-LF post", func() bool {
		mu.Lock()
		defer mu.Unlock()
		for _, a := range arts {
			if h, _ := splitArticle(a); headerValue(h, "Message-ID") == id {
				got = slices.Clone(a)
				return true
			}
		}
		return false
	})
	h, body := splitArticle(got)
	if v := headerValue(h, "Subject"); v != "all bare LF" {
		t.Errorf("Subject at the peer %q, want %q (article %q)", v, "all bare LF", got)
	}
	if !slices.Equal(body, []string{"body line"}) {
		t.Errorf("body at the peer %q", body)
	}
	mu.Lock()
	n := len(arts)
	mu.Unlock()
	if n != 1 {
		t.Errorf("the outgoing server got %d articles, want only the bare-LF one", n)
	}
}

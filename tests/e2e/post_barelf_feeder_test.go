package e2e

import (
	"bytes"
	"strings"
	"sync"
	"testing"
	"time"

	testkit "github.com/go-while/nntp-testkit"
	"github.com/go-while/nntp-testkit/fakeserver"
)

// postToFeederSite returns a reader site whose outgoing (post) server is a real diablo, and
// that diablo: dreaderd offers each POST there with IHAVE from 127.0.0.1, which the label
// "reader" lets in.
func postToFeederSite(t *testing.T) (*Site, *Site) {
	t.Helper()
	feeds := DefaultNewsfeeds + "label reader\n    inhost 127.0.0.1\n    addgroup *\nend\n"
	feeder := NewSite(t, SiteOptions{Newsfeeds: feeds})
	_, spool := spoolStore(t)
	port := strings.TrimPrefix(feeder.Addr, "127.0.0.1:")
	hosts := "127.0.0.1\ts1p" + strings.TrimPrefix(spool.Addr(), "127.0.0.1:") + "\n" +
		"127.0.0.1\to1p" + port + "\n"
	return NewReaderSite(t, ReaderOptions{Spool: spool, Files: map[string]string{"dserver.hosts": hosts}}), feeder
}

// postReply POSTs raw (the final dot line is added, ".\r\n" or ".\n" as dot says) on a new
// connection and waits up to d for the reply; code 0 means no reply came.
func postReply(t *testing.T, s *Site, raw, dot string, d time.Duration) (int, string) {
	t.Helper()
	c := s.Reader(t)
	defer c.Close()
	c.ExpectCmd(340, "POST")
	c.MustSend(raw + dot)
	c.Deadline = d
	code, line, err := c.ReadStatus()
	if err != nil {
		t.Logf("POST reply: %v", err)
		return 0, line
	}
	return code, line
}

// feederBody returns the body of article id as the diablo feeder serves it (ARTICLE on a peer
// connection), waiting up to 15 s for the article to be there.
func feederBody(t *testing.T, feeder *Site, id string) []string {
	t.Helper()
	p := feeder.Peer(t)
	defer p.Close()
	// a line may keep a CR of its own ("crcr\r\r\n"): read leniently, every CR before the
	// LF is stripped (TestPostLineEndsToPostServer checks the exact bytes)
	p.Strict = false
	var art []string
	testkit.WaitFor(t, 15*time.Second, "the feeder to serve "+id, func() bool {
		code, _, err := p.TryCmd("ARTICLE %s", id)
		if err != nil {
			t.Fatalf("ARTICLE %s at the feeder: %v", id, err)
		}
		if code != 220 {
			return false
		}
		art = p.ReadDotLines()
		return true
	})
	_, body := splitArticle(art)
	return body
}

// TestPostBareLFLastLineToFeeder (deep review r2 H1, D-225): a POST whose last body line
// ends in a bare LF reached the post server with that line as it was, followed by ".\r\n".
// diablo ends an article only on CRLF "." CRLF, so it took the "." as data and waited for
// more: the post connection never got an answer, the client never got its reply, and every
// later POST of that reader process queued behind it. dreaderd now passes every line of a
// post on ended by CRLF, the body as well as the headers. TestPostBareLFAccepted did not see
// it: its fake post server ends the article on any ".".
//
// Each case checks the reply (240 within 15 s) and the body the feeder then serves: the
// line contents are unchanged, only the line ends are CRLF.
func TestPostBareLFLastLineToFeeder(t *testing.T) {
	s, feeder := postToFeederSite(t)
	t.Cleanup(func() {
		if t.Failed() {
			t.Logf("reader general.log:\n%s", generalLog(s))
		}
	})
	head := func(id, eol string) string {
		return strings.Join([]string{
			"From: a <a@example.invalid>",
			"Newsgroups: e2e.test",
			"Subject: to a feeder",
			"Message-ID: " + id,
			"",
		}, eol) + eol
	}

	for _, tc := range []struct {
		name string
		raw  func(id string) string
		dot  string
		want []string
	}{
		// control: CRLF line ends throughout, diablo takes the post
		{"crlf", func(id string) string {
			return head(id, "\r\n") + "first body line\r\nlast body line\r\n"
		}, ".\r\n", []string{"first body line", "last body line"}},
		{"last line bare LF", func(id string) string {
			return head(id, "\r\n") + "first body line\r\nlast body line\n"
		}, ".\r\n", []string{"first body line", "last body line"}},
		{"every line bare LF, dot bare LF", func(id string) string {
			return head(id, "\n") + "first body line\nlast body line\n"
		}, ".\n", []string{"first body line", "last body line"}},
		{"bare LF blank, middle and dot-stuffed lines", func(id string) string {
			return head(id, "\r\n") + "first\n\nthird\r\n..dot line\n"
		}, ".\r\n", []string{"first", "", "third", ".dot line"}},
		{"one body line, bare LF", func(id string) string {
			return head(id, "\r\n") + "only line\n"
		}, ".\r\n", []string{"only line"}},
		// a line that ends in CR CR LF already ends in CR LF: passed on as it is (the reader
		// strips every CR before the LF, so the feeder's copy reads "crcr")
		{"last line CR CR LF", func(id string) string {
			return head(id, "\r\n") + "first\r\ncrcr\r\r\n"
		}, ".\r\n", []string{"first", "crcr"}},
		// a CR inside a line stays; the bare LF at its end gets its CR
		{"last line with a lone CR, bare LF", func(id string) string {
			return head(id, "\r\n") + "first\r\nlone\rcr\n"
		}, ".\r\n", []string{"first", "lone\rcr"}},
	} {
		id := msgID(t)
		code, line := postReply(t, s, tc.raw(id), tc.dot, 15*time.Second)
		if code != 240 {
			// on the stuck post connection every later POST would wait too
			t.Fatalf("%s: POST reply %d %q, want 240 within 15 s", tc.name, code, line)
		}
		if body := feederBody(t, feeder, id); strings.Join(body, "|") != strings.Join(tc.want, "|") {
			t.Errorf("%s: body at the feeder %q, want %q", tc.name, body, tc.want)
		}
	}

	// the post connection still works for the next client
	id := msgID(t)
	if code, line := postReply(t, s, head(id, "\r\n")+"after\r\n", ".\r\n", 15*time.Second); code != 240 {
		t.Fatalf("CRLF post after them: %d %q, want 240 within 15 s", code, line)
	}
	if body := feederBody(t, feeder, id); strings.Join(body, "|") != "after" {
		t.Errorf("CRLF post after them: body at the feeder %q, want [after]", body)
	}
}

// TestPostLineEndsToPostServer: the exact bytes dreaderd sends to the post server for a POST
// with mixed line ends. Every line ends in CR LF there: a line the client ended with CR LF
// (also CR CR LF) is passed on as it is, a bare LF (also after a lone CR inside the line, and
// on a blank or dot-stuffed line) gets a CR in front of it. Nothing else changes.
func TestPostLineEndsToPostServer(t *testing.T) {
	_, spool := spoolStore(t)
	var mu sync.Mutex
	raws := map[string][]byte{}
	post := fakeserver.New(t, fakeserver.Options{Handler: fakeserver.PeerHandler(&fakeserver.PeerOptions{
		OnArticleRaw: func(id string, raw []byte) {
			mu.Lock()
			defer mu.Unlock()
			raws[id] = append([]byte(nil), raw...)
		},
	})})
	s := NewReaderSite(t, ReaderOptions{Spool: spool, Post: post})

	id := msgID(t)
	in := "plain\r\nbare\n\nblank above\r\ncrcr\r\r\nlone\rcr\n..dot\nlast\n"
	want := "plain\r\nbare\r\n\r\nblank above\r\ncrcr\r\r\nlone\rcr\r\n..dot\r\nlast\r\n"
	raw := "From: a <a@example.invalid>\r\nNewsgroups: e2e.test\r\nSubject: line ends\r\nMessage-ID: " + id + "\r\n\r\n" + in
	if code, line := postReply(t, s, raw, ".\r\n", 15*time.Second); code != 240 {
		t.Fatalf("POST reply %d %q, want 240", code, line)
	}
	var got []byte
	testkit.WaitFor(t, 15*time.Second, "the post server to get "+id, func() bool {
		mu.Lock()
		defer mu.Unlock()
		got = raws[id]
		return got != nil
	})
	i := bytes.Index(got, []byte("\r\n\r\n"))
	if i < 0 {
		t.Fatalf("no blank line after the headers at the post server: %q", got)
	}
	body := strings.TrimSuffix(string(got[i+4:]), ".\r\n")
	if body != want {
		t.Errorf("body bytes at the post server %q, want %q", body, want)
	}
}

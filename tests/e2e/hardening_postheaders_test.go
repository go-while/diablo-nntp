package e2e

// dreaderd: the header checks of a POST apply to every header line, whatever
// the line end.

import (
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-while/nntp-testkit/fakeserver"
)

// postHeadersSite returns a reader site whose outgoing server records the articles it gets.
func postHeadersSite(t *testing.T, access string) (*Site, func() [][]string) {
	t.Helper()
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
	s := NewReaderSite(t, ReaderOptions{Spool: spool, Post: post, Access: access})
	return s, func() [][]string {
		mu.Lock()
		defer mu.Unlock()
		return append([][]string(nil), arts...)
	}
}

// postRaw POSTs the bytes raw (the final dot line is added) and returns the reply code.
func postRaw(t *testing.T, s *Site, raw string) int {
	t.Helper()
	c := s.Reader(t)
	defer c.Close()
	expect(t, c, 340, "POST")
	send(t, c, raw+".\r\n")
	code, line, err := c.ReadStatus()
	if err != nil {
		t.Fatalf("POST reply: %v", err)
	}
	t.Logf("reply %q", line)
	return code
}

// TestPostBareLFHeaders: a header that is refused when its lines end in CR LF is refused as
// well when the lines end in LF alone, also when the bare LF falls inside a run of headers.
func TestPostBareLFHeaders(t *testing.T) {
	access := strings.Replace(DefaultReaderAccess, "    post yes\n", "    post yes\n    controlpost no\n", 1)
	s, got := postHeadersSite(t, access)
	for i, h := range []string{
		"X-Trace: forged",
		"NNTP-Posting-Host: forged.example",
		"Control: newgroup e2e.zzz",
	} {
		head := func(sep string) string {
			return "From: a <a@example.invalid>" + "\r\n" + "Newsgroups: e2e.test" + "\r\n" +
				"Subject: s" + sep + h + sep + "Date: Mon, 02 Jan 2006 15:04:05 -0700" + "\r\n" +
				"Message-ID: <hardening-bare" + string(rune('a'+i)) + "@e2e.test>\r\n\r\nbody\r\n"
		}
		if code := postRaw(t, s, head("\r\n")); code != 441 {
			t.Fatalf("%q with CRLF: reply %d, want 441", h, code)
		}
		if code := postRaw(t, s, head("\n")); code != 441 {
			t.Errorf("%q after a bare LF: reply %d, want 441", h, code)
		}
	}
	if arts := got(); len(arts) > 0 {
		t.Errorf("the outgoing server got %d article(s), want none: %q", len(arts), arts[0])
	}
}

// TestPostBareLFAccepted: a post whose lines all end in LF alone, headers and body, still
// works; the headers reach the outgoing server as separate lines and the body is unchanged.
func TestPostBareLFAccepted(t *testing.T) {
	s, got := postHeadersSite(t, DefaultReaderAccess)
	id := msgID(t)
	raw := "From: a <a@example.invalid>\nNewsgroups: e2e.test\nSubject: bare line ends\nMessage-ID: " + id +
		"\n\nfirst body line\nsecond body line\n"
	if code := postRaw(t, s, raw); code != 240 {
		t.Fatalf("reply %d, want 240", code)
	}
	waitFor(t, 15*time.Second, "the outgoing server to receive the post", func() bool { return len(got()) > 0 })
	head, body := splitArticle(got()[0])
	if v := headerValue(head, "Subject"); v != "bare line ends" {
		t.Errorf("Subject at the peer %q", v)
	}
	if v := headerValue(head, "Message-ID"); v != id {
		t.Errorf("Message-ID at the peer %q, want %q", v, id)
	}
	if want := []string{"first body line", "second body line"}; strings.Join(body, "|") != strings.Join(want, "|") {
		t.Errorf("body at the peer %q, want %q", body, want)
	}
}

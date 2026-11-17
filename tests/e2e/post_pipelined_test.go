package e2e

import (
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-while/nntp-testkit/fakeserver"
)

// TestPostPipelinedCommandWhileQueued (deep review r2 H5): after the "." of a POST,
// dreaderd hands the article to the post server but leaves the client in the article state
// (co_Func stays NNAcceptArticle, no NNWaitThread). A command the client sends before its
// 240 is read as one more article line: it is appended to the article that is still on its
// way to the post server, and the command itself is never answered. The post server here
// waits 5 s before its 335, and the client sends the command 1 s after the ".", so it is
// read while the post is in flight.
func TestPostPipelinedCommandWhileQueued(t *testing.T) {
	_, spool := spoolStore(t)
	var mu sync.Mutex
	var arts [][]string
	post := fakeserver.New(t, fakeserver.Options{Handler: fakeserver.PeerHandler(&fakeserver.PeerOptions{
		OnArticle: func(id string, wire []string) {
			mu.Lock()
			defer mu.Unlock()
			arts = append(arts, wire)
		},
		BeforeReply: func(_ *fakeserver.Conn, cmd, _, status string) {
			if cmd == "IHAVE" && strings.HasPrefix(status, "335") {
				time.Sleep(5 * time.Second)
			}
		},
	})})
	s := NewReaderSite(t, ReaderOptions{Spool: spool, Post: post})

	id := msgID(t)
	body := []string{"first body line", "last body line"}
	c := s.Reader(t)
	defer c.Close()
	c.ExpectCmd(340, "POST")
	c.MustSend("From: a <a@example.invalid>\r\nNewsgroups: e2e.test\r\nSubject: pipelined\r\nMessage-ID: " + id +
		"\r\n\r\n" + strings.Join(body, "\r\n") + "\r\n.\r\n")
	// in its own write while the post waits for the 335: a line in the same write as the "."
	// stays in the input buffer until the 240, the new data wakes the client's descriptor
	time.Sleep(time.Second)
	c.MustSend("GROUP e2e.test\r\n")
	c.Deadline = 20 * time.Second
	if code, line, err := c.ReadStatus(); code != 240 {
		t.Errorf("POST reply %d %q (%v), want 240", code, line, err)
	}
	c.Deadline = 10 * time.Second
	if code, line, err := c.ReadStatus(); code != 211 {
		t.Errorf("reply to the GROUP sent after the post's dot: %d %q (%v), want 211", code, line, err)
	}

	var got []string
	ok := false
	for limit := time.Now().Add(15 * time.Second); time.Now().Before(limit); time.Sleep(100 * time.Millisecond) {
		mu.Lock()
		if len(arts) > 0 {
			got, ok = arts[0], true
		}
		mu.Unlock()
		if ok {
			break
		}
	}
	if !ok {
		t.Fatalf("the post server got no article")
	}
	if _, b := splitArticle(got); strings.Join(b, "|") != strings.Join(body, "|") {
		t.Errorf("body at the post server %q, want %q", b, body)
	}
}

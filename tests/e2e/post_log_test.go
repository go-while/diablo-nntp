package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	testkit "github.com/go-while/nntp-testkit"
	"github.com/go-while/nntp-testkit/client"
	"github.com/go-while/nntp-testkit/fakeserver"
)

// postLogSite is a reader with an outgoing server (refusing at IHAVE the ids in have and
// after the article the ids in reject) and the given diablo.config lines.
func postLogSite(t *testing.T, access string, config map[string]string, have, reject map[string]bool) (*Site, func() int) {
	t.Helper()
	_, spool := spoolStore(t)
	var mu sync.Mutex
	n := 0
	post := fakeserver.New(t, fakeserver.Options{Handler: fakeserver.PeerHandler(&fakeserver.PeerOptions{
		Have:      have,
		Reject:    reject,
		OnArticle: func(string, []string) { mu.Lock(); n++; mu.Unlock() },
	})})
	s := NewReaderSite(t, ReaderOptions{Spool: spool, Post: post, Access: access, Config: config})
	return s, func() int { mu.Lock(); defer mu.Unlock(); return n }
}

// postLogPost posts an article with these extra header lines and returns POST's reply code.
func postLogPost(t *testing.T, s *Site, hdrs ...string) int {
	t.Helper()
	c := s.Reader(t)
	defer c.Close()
	c.ExpectCmd(340, "POST")
	c.MustSend(client.Stuff(append(append([]string{
		"From: E2E Poster <poster@example.invalid>",
		"Newsgroups: e2e.test",
		"Subject: post log",
	}, hdrs...), "", "body")))
	code, _, err := c.ReadStatus()
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	return code
}

func readLog(s *Site, name string) string {
	b, _ := os.ReadFile(filepath.Join(s.Dir, "log", name))
	return string(b)
}

// logLine waits for a line of log name that contains every one of want and returns it.
func logLine(t *testing.T, s *Site, name string, want ...string) string {
	t.Helper()
	var found string
	testkit.WaitFor(t, 10*time.Second, name+" line with "+strings.Join(want, ", "), func() bool {
		for _, l := range strings.Split(readLog(s, name), "\n") {
			ok := true
			for _, w := range want {
				ok = ok && strings.Contains(l, w)
			}
			if ok {
				found = l
				return true
			}
		}
		return false
	})
	return found
}

// TestReaderPostLog: with path_postlog, dreaderd writes its "post ok" lines and, new, a
// "post refused <msgid> <reply>" line for every post it refuses (its own checks, POST not
// allowed) or that the server it posts to refuses (at IHAVE or after the article) into that
// file instead of the general log. Without path_postlog the lines stay in the general log.
// Before, there was no path_postlog, and refused posts were not logged at all.
func TestReaderPostLog(t *testing.T) {
	logs := map[string]string{"path_generallog": "%s/general.log", "path_postlog": "%s/post.log"}

	t.Run("post log", func(t *testing.T) {
		held, rejected := msgID(t), msgID(t)
		s, arts := postLogSite(t, DefaultReaderAccess, logs, map[string]bool{held: true}, map[string]bool{rejected: true})

		ok := msgID(t)
		if code := postLogPost(t, s, "Message-ID: "+ok); code != 240 {
			t.Fatalf("POST %s: %d, want 240", ok, code)
		}
		testkit.WaitFor(t, 10*time.Second, "the outgoing server to get the post", func() bool { return arts() > 0 })
		logLine(t, s, "post.log", "post ok "+ok)

		bad := msgID(t)
		if code := postLogPost(t, s, "Message-ID: "+bad, "Xref: e2e.test e2e.test:1"); code != 441 {
			t.Errorf("POST with an Xref: %d, want 441", code)
		}
		logLine(t, s, "post.log", "post refused "+bad, "441 Xref header not allowed in a post")

		if code := postLogPost(t, s, "Message-ID: "+held); code == 240 {
			t.Errorf("POST of an id the server has: 240, want a refusal")
		}
		logLine(t, s, "post.log", "post refused "+held, "435")

		if code := postLogPost(t, s, "Message-ID: "+rejected); code == 240 {
			t.Errorf("POST of an id the server rejects: 240, want a refusal")
		}
		logLine(t, s, "post.log", "post refused "+rejected, "437")

		for _, l := range strings.Split(readLog(s, "post.log"), "\n") {
			if l != "" && !strings.Contains(l, "127.0.0.1") {
				t.Errorf("post.log line without the client's address: %q", l)
			}
		}
		if g := readLog(s, "general.log"); strings.Contains(g, " post ok ") || strings.Contains(g, " post refused ") {
			t.Errorf("general.log has post lines with path_postlog set:\n%s", g)
		}
	})

	t.Run("posting not allowed", func(t *testing.T) {
		access := strings.Replace(DefaultReaderAccess, "    post yes\n", "    post no\n", 1)
		s, _ := postLogSite(t, access, logs, nil, nil)
		c := s.Reader(t)
		c.ExpectCmd(440, "POST")
		c.Close()
		logLine(t, s, "post.log", "post refused - 440 Posting Not Allowed.")
	})

	t.Run("no path_postlog", func(t *testing.T) {
		s, arts := postLogSite(t, DefaultReaderAccess, map[string]string{"path_generallog": "%s/general.log"}, nil, nil)
		ok := msgID(t)
		if code := postLogPost(t, s, "Message-ID: "+ok); code != 240 {
			t.Fatalf("POST %s: %d, want 240", ok, code)
		}
		testkit.WaitFor(t, 10*time.Second, "the outgoing server to get the post", func() bool { return arts() > 0 })
		logLine(t, s, "general.log", "post ok "+ok)
		bad := msgID(t)
		postLogPost(t, s, "Message-ID: "+bad, "Xref: e2e.test e2e.test:1")
		logLine(t, s, "general.log", "post refused "+bad)
		if _, err := os.Stat(filepath.Join(s.Dir, "log", "post.log")); err == nil {
			t.Errorf("post.log exists without path_postlog")
		}
	})
}

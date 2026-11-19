package e2e

import (
	"regexp"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	testkit "github.com/go-while/nntp-testkit"
	"github.com/go-while/nntp-testkit/client"
	"github.com/go-while/nntp-testkit/fakeserver"
)

var closedCounts = regexp.MustCompile(` posts=(-?[0-9]+) postbytes=(-?[0-9]+) postfail=(-?[0-9]+) `)

// sessionCounts waits for the "closed from" line of the session of c in the general log and
// returns its posts=, postbytes= and postfail=. The line names the client's address and port
// in parentheses, "(127.0.0.1:<port>)". The caller has sent QUIT.
func sessionCounts(t *testing.T, s *Site, c *client.Conn) (posts, postBytes, postFail int64) {
	t.Helper()
	tag := "(" + c.Conn.LocalAddr().String() + ")"
	var m []string
	var line string
	testkit.WaitFor(t, 15*time.Second, `the "closed from" line of the session `+tag+" in the general log", func() bool {
		for _, l := range strings.Split(generalLog(s), "\n") {
			if strings.Contains(l, "closed from") && strings.Contains(l, tag) {
				if m = closedCounts.FindStringSubmatch(l); m != nil {
					line = l
					return true
				}
			}
		}
		return false
	})
	n := func(i int) int64 {
		v, err := strconv.ParseInt(m[i], 10, 64)
		if err != nil {
			t.Fatalf("%q: %v", line, err)
		}
		return v
	}
	t.Logf("%s", line)
	return n(1), n(2), n(3)
}

// TestReaderPostBytesCountedOnce: an accepted POST is counted once in the post bytes of its
// session, "postbytes=" of the "closed from" log line (the POST total of "drcmd stats" adds up
// the postbytes of the closed client sessions). dreaderd adds the length of each line it reads
// to the count, and NNPostBuffer() added the size of the stored article to it once more when
// the checks passed, so a post that passed them showed its lines plus the stored article, also
// when the outgoing server refused it afterwards (that subtest checks postbytes= only). A post
// refused by dreaderd's own checks and a fed article were counted once.
//
// What the count is: the bytes of the lines the client sends after the POST command, up to but
// not including the "." line, as sent (CR LF line ends, a dot-stuffed line with its extra dot).
// For a fed article (IHAVE) the extra dot of a stuffed line is taken off before it is counted.
// A Bytes: or Xref: header line would be left out; the articles here have none, and no line
// is added, so each expected number is exact: the length of the article's lines plus 2 for
// each CR LF. It is not the size in the "post ok <id> <size>" line of the post log, which is
// the stored article with the headers dreaderd added.
func TestReaderPostBytesCountedOnce(t *testing.T) {
	_, spool := spoolStore(t)
	var offered atomic.Int32
	rejected := msgID(t)
	post := fakeserver.New(t, fakeserver.Options{Handler: fakeserver.PeerHandler(&fakeserver.PeerOptions{
		Reject:    map[string]bool{rejected: true},
		OnArticle: func(id string, wire []string) { offered.Add(1) },
	})})
	s := NewReaderSite(t, ReaderOptions{Spool: spool, Post: post})
	t.Cleanup(func() {
		if t.Failed() {
			t.Logf("general.log:\n%s", generalLog(s))
		}
	})

	// the bytes of lines as they are read for a POST (stuffed as sent) or for a fed article
	// (a leading extra dot taken off), each with its CR LF
	postWant := func(lines []string) int64 {
		return int64(len(client.Stuff(lines)) - len(".\r\n"))
	}
	fedWant := func(lines []string) int64 {
		var n int64
		for _, l := range lines {
			n += int64(len(l) + 2)
		}
		return n
	}
	check := func(t *testing.T, what string, posts, postBytes, postFail, wantPosts, wantBytes, wantFail int64) {
		t.Helper()
		if posts != wantPosts || postFail != wantFail {
			t.Errorf("%s: posts=%d postfail=%d, want posts=%d postfail=%d", what, posts, postFail, wantPosts, wantFail)
		}
		if postBytes != wantBytes {
			t.Errorf("%s: postbytes=%d, want %d (the bytes of its lines, counted once)", what, postBytes, wantBytes)
		}
	}

	t.Run("accepted post", func(t *testing.T) {
		id := msgID(t)
		lines := []string{
			"From: E2E Poster <poster@example.invalid>",
			"Newsgroups: e2e.test",
			"Subject: post bytes",
			"Message-ID: " + id,
			"",
			"posted body",
			".dot line",
		}
		c := s.Reader(t)
		defer c.Close()
		c.ExpectCmd(340, "POST")
		c.MustSend(client.Stuff(lines))
		c.ExpectStatus(240, "POST article")
		c.ExpectCmd(205, "QUIT")
		posts, postBytes, postFail := sessionCounts(t, s, c)
		check(t, "accepted post", posts, postBytes, postFail, 1, postWant(lines), 0)
		if postBytes != postWant(lines) {
			m := regexp.MustCompile(`post ok ` + regexp.QuoteMeta(id) + ` ([0-9]+)`).FindStringSubmatch(generalLog(s))
			t.Logf("the post log has %v for the stored article; the lines posted are %d bytes", m, postWant(lines))
		}
		if n := offered.Load(); n != 1 {
			t.Errorf("the outgoing server got %d articles, want 1", n)
		}
	})

	t.Run("refused post", func(t *testing.T) {
		lines := []string{
			"From: E2E Poster <poster@example.invalid>",
			"Newsgroups: e2e.test",
			"Newsgroups: e2e.test",
			"Subject: post bytes refused",
			"Message-ID: " + msgID(t),
			"",
			"posted body",
		}
		c := s.Reader(t)
		defer c.Close()
		c.ExpectCmd(340, "POST")
		c.MustSend(client.Stuff(lines))
		code, line, err := c.ReadStatus()
		if err != nil || code != 441 || !strings.Contains(line, "More than one Newsgroups header") {
			t.Fatalf("POST with two Newsgroups: lines: %d %q (%v), want 441 More than one Newsgroups header", code, line, err)
		}
		c.ExpectCmd(205, "QUIT")
		posts, postBytes, postFail := sessionCounts(t, s, c)
		check(t, "refused post", posts, postBytes, postFail, 0, postWant(lines), 1)
	})

	// dreaderd's checks pass ("post ok", posts= is counted then), the outgoing server refuses
	// the article with 437 and the client gets 441; posts= and postfail= are not looked at
	// here, only that the lines are counted once
	t.Run("post refused by the server", func(t *testing.T) {
		lines := []string{
			"From: E2E Poster <poster@example.invalid>",
			"Newsgroups: e2e.test",
			"Subject: post bytes refused by the server",
			"Message-ID: " + rejected,
			"",
			"posted body",
		}
		c := s.Reader(t)
		defer c.Close()
		c.ExpectCmd(340, "POST")
		c.MustSend(client.Stuff(lines))
		code, line, err := c.ReadStatus()
		if err != nil || code != 441 || !strings.Contains(line, "437") {
			t.Fatalf("POST of an article the server refuses: %d %q (%v), want 441 with the server's 437", code, line, err)
		}
		c.ExpectCmd(205, "QUIT")
		_, postBytes, _ := sessionCounts(t, s, c)
		if want := postWant(lines); postBytes != want {
			t.Errorf("post refused by the server: postbytes=%d, want %d (the bytes of its lines, counted once)", postBytes, want)
		}
	})

	t.Run("fed article accepted", func(t *testing.T) {
		id := msgID(t)
		lines := newArticle(id, "e2e.test", "fed bytes", "fed body", ".dot line")
		p := s.dialFrom(t, PeerIP)
		defer p.Close()
		p.ExpectCmd(335, "IHAVE %s", id)
		p.MustSend(client.Stuff(lines))
		p.ExpectStatus(235, "IHAVE "+id)
		p.ExpectCmd(205, "QUIT")
		posts, postBytes, postFail := sessionCounts(t, s, p)
		check(t, "fed article accepted", posts, postBytes, postFail, 1, fedWant(lines), 0)
	})

	t.Run("fed article refused", func(t *testing.T) {
		id := msgID(t)
		var lines []string
		for _, l := range newArticle(id, "e2e.test", "fed bytes refused", "fed body") {
			if !strings.HasPrefix(l, "Subject:") {
				lines = append(lines, l)
			}
		}
		p := s.dialFrom(t, PeerIP)
		defer p.Close()
		p.ExpectCmd(335, "IHAVE %s", id)
		p.MustSend(client.Stuff(lines))
		if code, line, err := p.ReadStatus(); err != nil || code != 437 {
			t.Fatalf("IHAVE without a Subject: header: %d %q (%v), want 437", code, line, err)
		}
		p.ExpectCmd(205, "QUIT")
		posts, postBytes, postFail := sessionCounts(t, s, p)
		check(t, "fed article refused", posts, postBytes, postFail, 0, fedWant(lines), 1)
	})
}

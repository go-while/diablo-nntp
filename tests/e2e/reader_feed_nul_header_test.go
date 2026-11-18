package e2e

// dreaderd: a header value with a NUL byte in it is freed in full. The copies of
// header values (lib/zalloc.c zallocStrTrim2(), zallocStrTrim()) did not stop at a NUL
// byte, so the copy and its allocation went on past the end of the C string, while the
// callers free a length of strlen() + 1 (zfreeStr() or a saved strlen() + 1): the bytes
// after the NUL stayed allocated in the connection's memory pool until the connection
// closed. A feed connection (Message-ID:, Subject:, Date:, Newsgroups:, Supersedes:, ...)
// and a reader connection that posts (Newsgroups:) grew with each such article, without
// limit. The copies now end at the NUL, where the C string the program uses them as ends.

import (
	"runtime"
	"strconv"
	"strings"
	"testing"
)

// nulTail returns head with the value of the first header called name followed by a NUL
// byte and tail.
func nulTail(t *testing.T, head []string, name, tail string) []string {
	t.Helper()
	for i, h := range head {
		if k, _, ok := strings.Cut(h, ":"); ok && strings.EqualFold(k, name) {
			head[i] = h + "\x00" + tail
			return head
		}
	}
	t.Fatalf("no %s: header in %q", name, head)
	return head
}

// TestReaderFeedNulHeaderMemory: over one feed connection, articles whose Message-ID:,
// Subject:, Date:, From: and Newsgroups: values go on after a NUL byte with 500 more bytes,
// and with 40 Supersedes: lines like that (the NUL also first in the value, or last in the
// line). The feed copies Message-ID:, Subject:, Date:, Newsgroups: and Supersedes:
// (dreaderd/group.c); the last Supersedes: has a value, so the cancel it runs copies From:
// and Newsgroups: once more (dreaderd/control.c). That is about 36 values with 500 bytes
// behind the NUL per article. After articles to settle, 800 more must not grow the
// private memory of dreaderd and its forks by more than 4 MiB; on main they grew by about
// 15 MB in our runs. Every article is still taken (235) and numbered in e2e.test (a plain
// article last, for OVER).
func TestReaderFeedNulHeaderMemory(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("reads /proc")
	}
	const (
		settle  = 200
		lines   = 40
		limitKB = 4 * 1024
	)
	tail := strings.Repeat("x", 500)
	nul := func(i int, id string, head []string) []string {
		for _, name := range []string{"Message-ID", "Subject", "Date", "From", "Newsgroups"} {
			head = nulTail(t, head, name, tail)
		}
		for k := range lines {
			old := "<old." + strconv.Itoa(i) + "." + strconv.Itoa(k) + "@e2e.test>"
			switch k % 4 {
			case 0: // the NUL first: an empty value
				head = append(head, "Supersedes: \x00"+old+tail)
			case 1: // the NUL last in the line
				head = append(head, "Supersedes: "+old+"\x00")
			default:
				head = append(head, "Supersedes: "+old+"\x00"+tail)
			}
		}
		return head
	}

	m := newFeedMemSite(t, nil)
	m.feed(t, settle, true, nul)
	m.part(t, "header values that go on after a NUL byte", 800, limitKB, true, nul)
	// OVER of an article with a NUL in its headers stops at the first NUL (the stored
	// article is kept as it came, on main too): the check reads a plain last article.
	m.feed(t, 1, true, func(i int, id string, head []string) []string { return head })
	m.check(t)
}

// TestReaderPostNulHeaderMemory: over one reader connection, posts with 40 Newsgroups:
// lines each whose value goes on after a NUL byte with 500 more bytes (refused: "441 More
// than one Newsgroups header"; dreaderd copies each value twice while it checks the post).
// After posts to settle, 400 more must not grow the private memory of dreaderd and its
// forks by more than 4 MiB; without the fix they keep about 40 x 2 x 500 bytes per post
// (about 16 MB). Nothing reaches the outgoing server.
func TestReaderPostNulHeaderMemory(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("reads /proc")
	}
	const (
		settle  = 100
		count   = 400
		lines   = 40
		limitKB = 4 * 1024
	)
	s, got := postHeadersSite(t, DefaultReaderAccess)
	c := s.Reader(t)
	tail := strings.Repeat("x", 500)
	post := func(n int) {
		t.Helper()
		for i := range n {
			var b strings.Builder
			b.WriteString("From: a <a@example.invalid>\r\n")
			for range lines {
				b.WriteString("Newsgroups: e2e.test\x00" + tail + "\r\n")
			}
			b.WriteString("Subject: post nul " + strconv.Itoa(i) + "\r\n")
			b.WriteString("Date: " + rfcDate(0) + "\r\n")
			b.WriteString("Message-ID: " + msgID(t) + "\r\n\r\nbody\r\n.\r\n")
			c.ExpectCmd(340, "POST")
			c.MustSend(b.String())
			code, line, err := c.ReadStatus()
			if err != nil {
				t.Fatalf("POST reply: %v", err)
			}
			// the refusal of the second Newsgroups: line: the header loop that copies the
			// values ran over them (dreaderd/post.c)
			if code != 441 || !strings.Contains(line, "More than one Newsgroups header") {
				t.Fatalf("POST with %d Newsgroups: lines: %q, want 441 More than one Newsgroups header", lines, line)
			}
		}
	}

	post(settle)
	r0 := feedTreeAnonKB(t, s.Proc.Pid())
	post(count)
	r1 := feedTreeAnonKB(t, s.Proc.Pid())
	t.Logf("RssAnon of dreaderd and its forks: %d kB before, %d kB after %d posts (%+d kB)", r0, r1, count, r1-r0)
	if d := r1 - r0; d > int64(limitKB) {
		t.Errorf("the reader's memory grew by %d kB over %d posts (limit %d kB, about %d bytes per post)", d, count, limitKB, d*1024/int64(count))
	}
	c.ExpectCmd(205, "QUIT")
	if arts := got(); len(arts) > 0 {
		t.Errorf("the outgoing server got %d article(s), want none: %q", len(arts), arts[0])
	}
}

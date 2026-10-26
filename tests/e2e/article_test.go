package e2e

import (
	"fmt"
	"maps"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-while/nntp-testkit/client"
)

var idSeq atomic.Int64

// msgID returns a Message-ID that no other test uses.
func msgID(tb testing.TB) string {
	name := strings.NewReplacer("/", ".", " ", "_").Replace(tb.Name())
	return fmt.Sprintf("<%s.%d.%d@%s>", name, time.Now().UnixNano(), idSeq.Add(1), PathHost)
}

// newArticle returns the lines of an article (head, blank line, body) as a peer would send
// it: Path, From, Newsgroups, Subject, Message-ID and Date.
func newArticle(id, newsgroups, subject string, body ...string) []string {
	head := []string{
		"Path: e2e.peer!not-for-mail",
		"From: E2E Test <e2e@example.invalid>",
		"Newsgroups: " + newsgroups,
		"Subject: " + subject,
		"Message-ID: " + id,
		"Date: " + time.Now().UTC().Format("Mon, 02 Jan 2006 15:04:05 -0700"),
	}
	return append(append(head, ""), body...)
}

// wire dot-stuffs lines and returns them with CRLF line ends and the terminating ".".
func wire(lines []string) string {
	var b strings.Builder
	for _, l := range lines {
		if strings.HasPrefix(l, ".") {
			b.WriteByte('.')
		}
		b.WriteString(l)
		b.WriteString("\r\n")
	}
	b.WriteString(".\r\n")
	return b.String()
}

// splitArticle returns the head and body of an article's lines (split at the first blank
// line).
func splitArticle(lines []string) (head, body []string) {
	i := slices.Index(lines, "")
	if i < 0 {
		return lines, nil
	}
	return lines[:i], lines[i+1:]
}

// headerValue returns the value of the first header field called name, or "".
func headerValue(head []string, name string) string {
	for _, h := range head {
		k, v, ok := strings.Cut(h, ":")
		if ok && strings.EqualFold(k, name) {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

// containsLine reports whether lines has a line equal to want.
func containsLine(lines []string, want string) bool { return slices.Contains(lines, want) }

// sortedKeys returns the keys of m in order.
func sortedKeys(m map[string]string) []string { return slices.Sorted(maps.Keys(m)) }

// expect sends a command and fails the test unless the reply code is want; it returns the
// status line.
func expect(tb testing.TB, c *client.Conn, want int, format string, args ...any) string {
	tb.Helper()
	code, line := c.Cmd(format, args...)
	if code != want {
		tb.Fatalf("%s: got %q, want %d", fmt.Sprintf(format, args...), line, want)
	}
	return line
}

// status reads one status line and fails the test unless its code is want.
func status(tb testing.TB, c *client.Conn, want int, what string) string {
	tb.Helper()
	code, line, err := c.ReadStatus()
	if err != nil {
		tb.Fatalf("%s: %v", what, err)
	}
	if code != want {
		tb.Fatalf("%s: got %q, want %d", what, line, want)
	}
	return line
}

// send writes raw bytes and fails the test on error.
func send(tb testing.TB, c *client.Conn, raw string) {
	tb.Helper()
	if err := c.Send(raw); err != nil {
		tb.Fatal(err)
	}
}

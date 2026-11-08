package e2e

import (
	"fmt"
	"maps"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

var idSeq atomic.Int64

// msgID returns a Message-ID that no other test uses.
func msgID(tb testing.TB) string {
	// Test names can contain punctuation or Unicode that is not dot-atom-text
	// (RFC 5536 3.1.3). Keep generated IDs valid under the default POST checks.
	name := strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' {
			return r
		}
		return '_'
	}, tb.Name())
	if len(name) > 180 { // room for the timestamp, counter and domain within 250 octets
		name = name[:180]
	}
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

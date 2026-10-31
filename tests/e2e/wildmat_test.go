package e2e

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/go-while/nntp-testkit/client"
)

// listActiveGroups sends LIST ACTIVE with pattern and returns the group names listed.
func listActiveGroups(t *testing.T, c *client.Conn, pattern string) []string {
	t.Helper()
	expect(t, c, 215, "LIST ACTIVE %s", pattern)
	var groups []string
	for _, l := range c.ReadDotLines() {
		if f := strings.Fields(l); len(f) > 0 {
			groups = append(groups, f[0])
		}
	}
	slices.Sort(groups)
	return groups
}

// TestReaderListWildcardTime: LIST ACTIVE with a pattern of many "*?" pairs against a long
// group name is answered at once, and a second client gets its greeting meanwhile. The old
// matcher retried every split at each '*' and took seconds to minutes on such a pattern,
// stalling every client of that reader process. The test also checks that '*' and '?' still
// match as before.
func TestReaderListWildcardTime(t *testing.T) {
	long := "e2e." + strings.Repeat("a", 52) // 56 characters
	_, spool := spoolStore(t)
	s := NewReaderSite(t, ReaderOptions{Spool: spool, Groups: []string{"e2e.test", "e2e.other", long}})

	c := s.Reader(t)
	for _, tc := range []struct {
		pattern string
		want    []string
	}{
		{"e2e.*", []string{long, "e2e.other", "e2e.test"}},
		{"e2e.?est", []string{"e2e.test"}},
		{"*r", []string{"e2e.other"}},
		{"*.*a", []string{long}},
		{"e2e.test", []string{"e2e.test"}},
		{"e2e.?", nil},
	} {
		if got := listActiveGroups(t, c, tc.pattern); !slices.Equal(got, tc.want) {
			t.Errorf("LIST ACTIVE %s: %q, want %q", tc.pattern, got, tc.want)
		}
	}

	// Eight "*?" pairs and a character no group has: no match, but the old matcher tries
	// every way to place the pairs in the 56 characters (several seconds or more).
	pattern := strings.Repeat("*?", 8) + "X"
	start := time.Now()
	send(t, c, "LIST ACTIVE "+pattern+"\r\n")

	const limit = 5 * time.Second
	c2, err := client.TryDial(s.Addr, limit)
	if err != nil {
		t.Fatalf("second client during LIST ACTIVE %s: %v (after %v)", pattern, err, time.Since(start))
	}
	c2 = c2.WithTB(t)
	c.Deadline = limit
	status(t, c, 215, "LIST ACTIVE "+pattern)
	if got := c.ReadDotLines(); len(got) != 0 {
		t.Errorf("LIST ACTIVE %s: %q, want no groups", pattern, got)
	}
	if d := time.Since(start); d > limit {
		t.Errorf("LIST ACTIVE %s took %v", pattern, d)
	}
	expect(t, c2, 205, "QUIT")
	expect(t, c, 205, "QUIT")
}

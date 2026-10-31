package e2e

import (
	"strings"
	"testing"
	"time"
)

// TestReaderListWildcardLinear: LIST ACTIVE with a pattern made of many '*' and '?' against a
// long group name is answered at once, and another client is served meanwhile. The matching
// of ordinary patterns is unchanged.
func TestReaderListWildcardLinear(t *testing.T) {
	_, spool := spoolStore(t)
	long := "e2e.abcdefghijklmnopqrstuvwxyzabcdefghijklmnopqrstuvwxyz"
	s := NewReaderSite(t, ReaderOptions{Spool: spool, Groups: []string{long}})

	c := s.Reader(t)
	for pat, want := range map[string]bool{
		"e2e.*":     true,
		"*.abc*xyz": true,
		"e2e.?bc*":  true,
		"e2e.?c*":   false,
		"*zz":       false,
		"e2e.abc":   false,
		long:        true,
		"*" + long:  true,
		"**":        true,
	} {
		expect(t, c, 215, "LIST ACTIVE %s", pat)
		got := len(c.ReadDotLines()) > 0
		if got != want {
			t.Errorf("LIST ACTIVE %s: matched=%v, want %v", pat, got, want)
		}
	}

	// a literal that is in no group name (the names contain a lowercase x)
	pat := strings.Repeat("*?", 22) + "X"
	send(t, c, "LIST ACTIVE "+pat+"\r\n")
	time.Sleep(2 * time.Second)
	// time the connect too: the greeting of the second client is what a stalled reader delays
	start := time.Now()
	c2 := s.Reader(t)
	expect(t, c2, 111, "DATE")
	if d := time.Since(start); d > 2*time.Second {
		t.Errorf("another client was blocked for %v", d)
	}
	code, line, err := c.ReadStatus()
	if err != nil || code != 215 {
		t.Fatalf("LIST ACTIVE with a long pattern: %d %q err=%v", code, line, err)
	}
	if lines := c.ReadDotLines(); len(lines) != 0 {
		t.Errorf("pattern matched %q", lines)
	}
}

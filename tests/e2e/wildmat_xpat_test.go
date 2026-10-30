package e2e

import (
	"strings"
	"testing"
	"time"
)

// GHSA-wildmat-xpat: an unterminated '[' character class in an XPAT pattern
// must not read past the end of the pattern.  lib/wildmat.c DoMatch() had two
// over-reads in the class arm:
//
//   - site 1: the class loop stops on the NUL, then the arm's continue makes
//     the outer for-increment read one byte past the NUL;
//   - site 2: a trailing '-' takes the range branch, whose *++p steps onto the
//     NUL, and the class loop then walks until a 0 or ']' byte.
//
// The fix treats an unterminated class as "no match".  This test drives the
// real XPAT path over the wire (GROUP, numeric range, pattern) and is a
// detector without an ASAN build:
//
//   - the article's Subject is exactly "a" and the pattern is "[a": the old
//     matcher reads the zero byte after the NUL and returns a match, so the
//     article is listed; the fixed matcher returns no match;
//   - "[a-" is the runaway: on the old code the reader child can die or the
//     connection can drop; the fixed code answers 221 and stays in step.
func TestReaderXPatUnterminatedClass(t *testing.T) {
	st, spool := spoolStore(t)
	s := NewReaderSite(t, ReaderOptions{Spool: spool})
	id := msgID(t)
	feedReader(t, s, st, "e2e.test", newArticle(id, "e2e.test", "a", "wildmat xpat body"))

	r := s.Reader(t)
	waitFor(t, 10*time.Second, "the article in GROUP e2e.test", func() bool {
		_, line := r.Cmd("GROUP e2e.test")
		f := strings.Fields(line)
		return len(f) > 1 && f[0] == "211" && f[1] != "0"
	})

	// The group is fresh: article 1 is the only one, so 1-1 selects it.
	for _, pat := range []string{"[a", "[a-"} {
		code, line := r.Cmd("XPAT subject 1-1 %s", pat)
		if code != 221 {
			t.Errorf("XPAT subject 1-1 %q: got %d %q, want 221", pat, code, line)
			continue
		}
		if got := r.ReadDotLines(); len(got) != 0 {
			t.Errorf("XPAT subject 1-1 %q: got %d match(es) %q, want none (unterminated class)", pat, len(got), got)
		}
	}

	// The reader child must still be alive and the connection in step.
	if code, line := r.Cmd("GROUP e2e.test"); code != 211 {
		t.Errorf("GROUP after XPAT: got %d %q, want 211", code, line)
	}
	expect(t, r, 205, "QUIT")
}

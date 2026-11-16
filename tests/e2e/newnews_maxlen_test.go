package e2e

import (
	"strings"
	"testing"
)

// TestReaderNewnewsMaxLen: with readernewnewsmaxlen set, a NEWNEWS whose newsgroups wildmat
// is longer than the limit gets "501 Argument too long" (as INN's nnrpd for an argument over
// 497 bytes) and the connection goes on; a wildmat of exactly the limit is served. Without
// the option (the default) a long wildmat is served as before.
func TestReaderNewnewsMaxLen(t *testing.T) {
	access := strings.Replace(DefaultReaderAccess, "readerdef rp\n", "readerdef rp\n    allownewnews yes\n", 1)
	// "e2e.test," pieces padded to an exact length with a last wildmat piece
	wildmat := func(n int) string {
		w := strings.Repeat("e2e.test,", (n-1)/9)
		return w + strings.Repeat("*", n-len(w))
	}
	if len(wildmat(497)) != 497 || len(wildmat(498)) != 498 {
		t.Fatalf("wildmat lengths: %d, %d", len(wildmat(497)), len(wildmat(498)))
	}

	t.Run("limit 497", func(t *testing.T) {
		_, spool := spoolStore(t)
		s := NewReaderSite(t, ReaderOptions{Spool: spool, Access: access,
			Config: map[string]string{"readernewnewsmaxlen": "497"}})
		c := s.Reader(t)
		defer c.Close()
		code, line, err := c.TryCmd("NEWNEWS %s 000101 000000 GMT", wildmat(498))
		if err != nil || code != 501 || !strings.Contains(line, "Argument too long") {
			t.Errorf("NEWNEWS with a 498-byte wildmat: %d %q %v, want 501 Argument too long", code, line, err)
		}
		c.ExpectCmd(230, "NEWNEWS %s 000101 000000 GMT", wildmat(497))
		c.ReadDotLines()
		c.ExpectCmd(111, "DATE")
	})

	t.Run("default", func(t *testing.T) {
		_, spool := spoolStore(t)
		s := NewReaderSite(t, ReaderOptions{Spool: spool, Access: access})
		c := s.Reader(t)
		defer c.Close()
		c.ExpectCmd(230, "NEWNEWS %s 000101 000000 GMT", wildmat(4000))
		c.ReadDotLines()
	})
}

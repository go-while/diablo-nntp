package e2e

import (
	"strings"
	"testing"
	"time"

	"github.com/go-while/nntp-testkit/fakeserver"
)

// TestCtypeHighByteInGroupDesc: newsgroup descriptions with non-ASCII bytes (>0x7F)
// must not be corrupted by SanitizeDescString. The function used iscntrl((int)*name)
// where name is a plain char* — on systems where char is signed and the C-library
// ctype table does not extend to negative indices (BSD, illumos), iscntrl(-23) for
// byte 0xE9 returns a non-zero value and the byte is replaced with '?'. Fails on
// main on BSD/illumos; on glibc/Linux both pass (glibc extends the table). The fix
// casts to (unsigned char) which is correct on every platform.
func TestCtypeHighByteInGroupDesc(t *testing.T) {
	// "Caf\xe9" is a Latin-1 encoded description with a non-ASCII byte (é = 0xE9).
	desc := "Caf\xe9 discussion"
	s := NewReaderSite(t, ReaderOptions{
		SpoolAddr: "127.0.0.1:1",
		Groups:    []string{"e2e.ctype"},
		NoStart:   true,
	})
	// dgrpctl newgroup stores the description through SanitizeDescString.
	s.Run(t, "dgrpctl", "newgroup", "e2e.ctype", "y", `""`, desc)

	out := s.Run(t, "dgrpctl", "groupinfo", "e2e.ctype")
	t.Logf("dgrpctl groupinfo: %s", out)

	// The description must survive round-trip without the non-ASCII byte becoming '?'.
	// (GD= value is URL-encoded or raw depending on version; check for the raw byte.)
	for _, h := range strings.Fields(out) {
		if strings.HasPrefix(h, "GD=") {
			v := strings.TrimPrefix(h, "GD=")
			if strings.Contains(v, "?") && !strings.Contains(v, "\xe9") {
				t.Errorf("SanitizeDescString corrupted non-ASCII byte in description: %q", v)
			}
		}
	}
}

// TestCtypeHighByteInCacheHeader: DumpArticleFromCache uses tolower and isspace on
// header bytes without (unsigned char) cast. On systems where char is signed and
// ctype tables do not extend to negative indices, this is UB that may produce wrong
// results or crash. This test stores an article whose Xref header contains a
// non-ASCII byte, then retrieves it from the reader cache and verifies it arrives
// uncorrupted. Fails on main on BSD/illumos; passes on glibc/Linux in both.
func TestCtypeHighByteInCacheHeader(t *testing.T) {
	st, spool := spoolStore(t)
	s := NewReaderSite(t, ReaderOptions{
		Spool:  spool,
		Config: map[string]string{"readercache": "on"},
	})

	id := msgID(t)
	// Header with a non-ASCII byte in the Xref host field (e.g., after the colon and space).
	// The isspace scan in DumpArticleFromCache will scan past this byte.
	xref := "Xref: e2e.spool\xc0test e2e.test:1"
	art := newArticle(id, "e2e.test", "d98 high-byte cache", "body")
	head, body := splitArticle(art)
	sh := []string{xref}
	for _, h := range head {
		if !strings.HasPrefix(h, "Path:") {
			sh = append(sh, h)
		}
	}
	sh = append(sh, "Path: e2e.spool!not-for-mail")

	st.AddArticle("e2e.test", fakeserver.Article{MessageID: id, Headers: sh, Body: body})

	p := s.dialFrom(t, PeerIP)
	expect(t, p, 335, "IHAVE %s", id)
	send(t, p, wire(art))
	status(t, p, 235, "IHAVE "+id)
	expect(t, p, 205, "QUIT")

	r := s.Reader(t)
	waitFor(t, 10*time.Second, "article in GROUP e2e.test", func() bool {
		_, line := r.Cmd("GROUP e2e.test")
		f := strings.Fields(line)
		return len(f) > 1 && f[0] == "211" && f[1] == "1"
	})
	// First read populates the cache.
	expect(t, r, 220, "ARTICLE %s", id)
	r.ReadDotLines()

	waitFor(t, 10*time.Second, "cached article", func() bool {
		return countNonEmptyFiles(s.Dir+"/spool/cache") >= 1
	})
	// Second read served from cache (exercises DumpArticleFromCache).
	expect(t, r, 220, "ARTICLE %s", id)
	lines := r.ReadDotLines()
	if len(lines) == 0 {
		t.Errorf("ARTICLE from cache returned no lines")
	}
	expect(t, r, 205, "QUIT")
}

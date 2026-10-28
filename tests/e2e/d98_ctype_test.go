package e2e

import (
	"io/fs"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-while/nntp-testkit/fakeserver"
)

// TestCtypeHighByteInGroupDesc: newsgroup descriptions with non-ASCII bytes (>0x7F)
// must not be corrupted by SanitizeDescString. The function used iscntrl((int)*name)
// where name is a plain char* — on systems where char is signed and the C-library
// ctype table does not extend to negative indices (BSD, illumos), iscntrl(-23) for
// byte 0xE9 returns a non-zero value and the byte is replaced with '?'. The fix
// casts to (unsigned char) which is correct on every platform.
//
// This is a strict-libc regression guard: on glibc/Linux the undefined behaviour of
// iscntrl((int)(signed char)0xE9) is benign (glibc extends the ctype table to cover
// negative char values), so both main and the fix pass here; on BSD/illumos with
// strict tables it fails on main.
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

	// dgrpctl groupinfo prints the stored GD field as is. KPDBWriteEncode (lib/kpdb.c)
	// writes every byte except [A-Za-z0-9.] as %xx in lower case hex, so the 0xE9 that
	// survived is "%e9", a space is "%20", and a '?' written by SanitizeDescString for a
	// byte it took for a control character is "%3f". A literal '?' never shows up in
	// the output. Without a GD field dgrpctl prints the default "GD=?".
	gd, found := "", false
	for _, f := range strings.Fields(out) {
		if v, ok := strings.CutPrefix(f, "GD="); ok {
			gd, found = strings.ToLower(v), true
		}
	}
	if !found || gd == "?" {
		t.Fatalf("no GD= field (description not stored) in dgrpctl groupinfo output:\n%s", out)
	}
	if strings.Contains(gd, "%3f") {
		t.Errorf("SanitizeDescString corrupted the non-ASCII byte 0xE9 into '?' (%%3f) in the description: GD=%s", gd)
	}
	if !strings.Contains(gd, "%e9") {
		t.Errorf("description lost the non-ASCII byte 0xE9 (want %%e9 in GD=): GD=%s", gd)
	}
}

// TestCtypeHighByteInCacheHeader: DumpArticleFromCache uses tolower and isspace on
// header bytes without (unsigned char) cast. On systems where char is signed and
// ctype tables do not extend to negative indices, this is UB that may produce wrong
// results or crash. This test stores an article whose Xref header contains a
// non-ASCII byte in the host token, reads it once (which fills the reader cache),
// stops the spool and reads it again, so only the cache can answer, and verifies the
// served Xref line.
//
// The 0xC0 byte itself is not in the served line: DumpArticleFromCache replaces the
// host token of Xref: with the vserver's cluster name (PathHost) and keeps the groups
// behind it. What the isspace scan over that token has to get right is where the token
// ends: if isspace(0xC0) were true, the token would end at the 0xC0 and the served
// line would be "Xref: <PathHost> test e2e.test:1" instead of "Xref: <PathHost>
// e2e.test:1". That exact line is what is checked.
//
// This is a strict-libc regression guard: on glibc/Linux the undefined behaviour of
// isspace((int)(signed char)0xC0) is benign (glibc extends the ctype table to cover
// negative char values), so both main and the fix pass here, and the test still
// catches a crash; on BSD/illumos with strict tables it fails on main.
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
	// First read: fetched from the spool, which fills the cache.
	expect(t, r, 220, "ARTICLE %s", id)
	r.ReadDotLines()

	cacheDir := filepath.Join(s.Dir, "spool", "cache")
	waitFor(t, 10*time.Second, "committed cache file in "+cacheDir, func() bool {
		return cacheCommitted(cacheDir)
	})

	// Stop the spool (an in-process fakeserver, closed with Close). NNArticleRetrieveByMessageId
	// looks in the cache before it asks the spool, so from here on only a cache hit can
	// answer; a miss is a 430 or a hang, not a 220. This exercises DumpArticleFromCache.
	spool.Close()
	code, line := r.Cmd("ARTICLE %s", id)
	if code != 220 {
		t.Fatalf("ARTICLE %s with the spool stopped: got %q, want 220 (not served from the cache)", id, line)
	}
	lines := r.ReadDotLines()

	gotHead, _ := splitArticle(lines)
	wantXref := "Xref: " + PathHost + " e2e.test:1"
	gotXref := ""
	for _, h := range gotHead {
		if strings.HasPrefix(h, "Xref:") {
			gotXref = h
		}
	}
	if gotXref != wantXref {
		t.Errorf("Xref from cache = %q, want %q (head %q)", gotXref, wantXref, gotHead)
	}
	expect(t, r, 205, "QUIT")
}

// cacheCommitted reports whether dir holds a finished cache file: a non-empty regular
// file that is not a ".tmp" file dreaderd is still writing (CommitCache renames it).
func cacheCommitted(dir string) bool {
	found := false
	filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err == nil && d.Type().IsRegular() && !strings.HasSuffix(p, ".tmp") {
			if fi, err := d.Info(); err == nil && fi.Size() > 0 {
				found = true
			}
		}
		return nil
	})
	return found
}

package e2e

import (
	"os"
	"strconv"
	"strings"
	"testing"
)

// TestStatWithLongSpoolPath (D-62): STAT of an article that exists must answer 223 when the spool
// path is long. Fails on main: with a spool path of 125 characters the file name did not fit
// the 128-byte buffer of STAT, which answered "430 Article expired" (HEAD worked).
func TestStatWithLongSpoolPath(t *testing.T) {
	long := "/tmp/" + strings.Repeat("e2e-long-spool-dir-", 6) + strconv.Itoa(os.Getpid())
	t.Cleanup(func() { os.RemoveAll(long) })
	s := NewSite(t, SiteOptions{
		Setup: func(tb testing.TB, s *Site) {
			if err := os.MkdirAll(long, 0o755); err != nil {
				tb.Fatal(err)
			}
		},
		SpoolCtl: `spool 01
    path ` + long + `
    minfree 1m
end
metaspool trad
    spool 01
end
expire * trad
`})
	p := s.Peer(t)
	id := msgID(t)
	expect(t, p, 335, "IHAVE %s", id)
	send(t, p, wire(newArticle(id, "e2e.test", "long spool path", "body")))
	status(t, p, 235, "IHAVE")
	if len(long) < 120 {
		t.Fatalf("spool path of %d characters is too short to test STAT's buffer", len(long))
	}
	expect(t, p, 223, "STAT %s", id)
	expect(t, p, 205, "QUIT")
}

package e2e

import (
	"github.com/go-while/nntp-testkit/fakeserver"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// dnlSpool creates the spool file D.00000001/B.0001 that the hand-written queue lines of
// the dnewslink tests name: 1 MB of NUL bytes, so that an "off,100" entry maps (a spool
// file has a NUL after each article). dnewslink drops an entry whose spool file is gone
// without offering it.
func dnlSpool(t *testing.T, s *Site) {
	t.Helper()
	dir := filepath.Join(s.Dir, "spool", "news", "D.00000001")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "B.0001"), make([]byte, 1<<20), 0o644); err != nil {
		t.Fatal(err)
	}
}

func dnlQueue(t *testing.T, s *Site, name string) string {
	b, err := os.ReadFile(s.Dir + "/dqueue/" + name)
	if err != nil {
		return "<missing: " + err.Error() + ">"
	}
	return string(b)
}

// D-66: articles that the peer defers (431 to CHECK) are kept only in dnewslink's memory. If the
// connection then ends without a reconnect (peer says 400 / goes away), they are not written
// back to the queue file and are lost.
func TestDnewslinkDeferredKeptOnExit(t *testing.T) {
	s := NewSite(t, SiteOptions{NoStart: true})
	dnlSpool(t, s)
	srv := fakeserver.New(t, fakeserver.Options{Greeting: "200 fake", Handler: fakeserver.LineHandler(func(_ *fakeserver.Conn, line string) ([]string, bool) {
		line = strings.ToLower(strings.TrimSpace(line))
		switch {
		case strings.HasPrefix(line, "mode stream"):
			return []string{"203 stream ok"}, false
		case strings.HasPrefix(line, "check <a1@") || strings.HasPrefix(line, "check <a2@"):
			return []string{"431 " + strings.Fields(line)[1] + " try later"}, false
		case strings.HasPrefix(line, "check <a3@"):
			return []string{"400 going away"}, true
		}
		return nil, false
	})})
	port := srv.Port()
	q := "D.00000001/B.0001 <a1@e2e.test> 0,100\nD.00000001/B.0001 <a2@e2e.test> 100,100\nD.00000001/B.0001 <a3@e2e.test> 200,100\n"
	if err := os.WriteFile(s.Dir+"/dqueue/out.S00000", []byte(q), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := s.TryRun("dnewslink", "-N", "1", "-S", "0", "-b", "out.S%05d", "-h", "127.0.0.1",
		"-P", strconv.Itoa(port), "-t", "5")
	t.Logf("dnewslink: err=%v\n%s", err, out)
	got := dnlQueue(t, s, "out.S00000")
	if !strings.HasSuffix(got, "\n") {
		t.Errorf("queue file does not end with a newline: %q", got)
	}
	// every entry on a line of its own, exactly once
	lines := strings.Split(strings.TrimSuffix(got, "\n"), "\n")
	for _, id := range []string{"<a1@e2e.test>", "<a2@e2e.test>", "<a3@e2e.test>"} {
		n := 0
		for _, l := range lines {
			if strings.Contains(l, id) {
				n++
				if strings.Count(l, "<") != 1 {
					t.Errorf("%s shares a queue line with another entry: %q", id, l)
				}
			}
		}
		if n != 1 {
			t.Errorf("%s is on %d queue lines, want 1; queue file now:\n%s", id, n, got)
		}
	}
}

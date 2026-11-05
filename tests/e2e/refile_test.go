package e2e

import (
	"github.com/go-while/nntp-testkit/fakeserver"
	"os"
	"strconv"
	"strings"
	"testing"
)

// TestDnewslinkRefileKeepsFlags (D-63): when the peer goes away with streamed entries still
// pending, dnewslink writes them back to the queue file. Each entry must come back once, with
// its ' H' (header only) and ' C<size>' (compressed spool) flags. Fails on main: the pending
// entries lost both flags, and the one being sent was written a second time with them.
func TestDnewslinkRefileKeepsFlags(t *testing.T) {
	s := NewSite(t, SiteOptions{NoStart: true})
	dnlSpool(t, s)
	srv := fakeserver.New(t, fakeserver.Options{Greeting: "200 fake", Handler: fakeserver.LineHandler(func(_ *fakeserver.Conn, line string) ([]string, bool) {
		line = strings.ToLower(strings.TrimSpace(line))
		switch {
		case strings.HasPrefix(line, "mode headfeed"):
			return []string{"250 ok"}, false
		case strings.HasPrefix(line, "mode stream"):
			return []string{"203 stream ok"}, false
		case strings.HasPrefix(line, "check"):
			return nil, true // go away with the CHECKs unanswered
		}
		return nil, false
	})})
	port := srv.Port()
	q := "D.00000001/B.0001 <a1@e2e.test> 0,100 H C50\nD.00000001/B.0001 <a2@e2e.test> 100,100 H C50\n"
	if err := os.WriteFile(s.Dir+"/dqueue/out.S00000", []byte(q), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := s.TryRun("dnewslink", "-H", "-N", "1", "-S", "0", "-b", "out.S%05d", "-h", "127.0.0.1",
		"-P", strconv.Itoa(port), "-t", "5")
	t.Logf("dnewslink: err=%v\n%s", err, out)

	b, rerr := os.ReadFile(s.Dir + "/dqueue/out.S00000")
	if rerr != nil {
		t.Fatal(rerr)
	}
	var lines []string
	for _, l := range strings.Split(string(b), "\n") {
		if strings.TrimSpace(l) != "" {
			lines = append(lines, l)
		}
	}
	if len(lines) != 2 {
		t.Fatalf("queue file has %d entries, want the 2 that were queued:\n%s", len(lines), b)
	}
	for i, id := range []string{"<a1@e2e.test>", "<a2@e2e.test>"} {
		f := strings.Fields(lines[i])
		if len(f) < 3 || f[1] != id || !strings.Contains(lines[i], " H") || !strings.Contains(lines[i], "C50") {
			t.Errorf("entry %d is %q, want %s with its H and C50 flags", i+1, lines[i], id)
		}
	}
}

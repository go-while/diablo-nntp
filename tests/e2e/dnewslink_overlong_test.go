package e2e

import (
	"github.com/go-while/nntp-testkit/fakeserver"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
)

// D-72: a line of 8 KB or more in a queue file (for example a hole of garbage after a crash) makes
// readline() fail for good; dnewslink treats that as the end of the file, sees nothing left to
// rewrite and removes the queue file with every entry behind the bad line.
func TestDnewslinkOverlongQueueLine(t *testing.T) {
	s := NewSite(t, SiteOptions{NoStart: true})
	dnlSpool(t, s)
	var mu sync.Mutex
	var taken []string
	srv := fakeserver.New(t, fakeserver.Options{Greeting: "200 fake", Handler: fakeserver.LineHandler(func(_ *fakeserver.Conn, line string) ([]string, bool) {
		line = strings.ToLower(strings.TrimSpace(line))
		if strings.HasPrefix(line, "mode stream") {
			return []string{"203 stream ok"}, false
		}
		if strings.HasPrefix(line, "check ") {
			return []string{"238 " + strings.Fields(line)[1]}, false
		}
		if strings.HasPrefix(line, "takethis ") {
			id := strings.Fields(line)[1]
			mu.Lock()
			taken = append(taken, id)
			mu.Unlock()
			return []string{"239 " + id}, false
		}
		return nil, false
	})})
	port := srv.Port()
	q := strings.Repeat("x", 9000) + "\n" +
		"D.00000001/B.0001 <b1@e2e.test> 0,100\nD.00000001/B.0001 <b2@e2e.test> 100,100\n"
	if err := os.WriteFile(s.Dir+"/dqueue/out.S00000", []byte(q), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := s.TryRun("dnewslink", "-N", "1", "-S", "0", "-b", "out.S%05d", "-h", "127.0.0.1",
		"-P", strconv.Itoa(port), "-t", "5")
	t.Logf("dnewslink: err=%v\n%s", err, out)
	mu.Lock()
	got := strings.Join(taken, " ")
	mu.Unlock()
	for _, id := range []string{"<b1@e2e.test>", "<b2@e2e.test>"} {
		if !strings.Contains(got, id) {
			t.Errorf("%s behind the long line was not sent (TAKETHIS seen for: %q)", id, got)
		}
	}
	if left := dnlQueue(t, s, "out.S00000"); strings.HasPrefix(left, "<missing") == false {
		t.Errorf("queue file should be gone after both entries were sent; it holds: %.200q", left)
	}
}

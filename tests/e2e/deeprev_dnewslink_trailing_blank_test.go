package e2e

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/go-while/nntp-testkit/fakeserver"
)

// TestDeepRevDnewslinkTrailingBlank (L3-10): the flag scan of a queue line skipped the blanks
// before a flag and then stepped over the next character without looking at it. For a line
// that ends in a blank that character is the end of the line, and the scan went on in what
// the line buffer still held of an earlier, longer line. Here the earlier line carries the
// header-only flag " H": the second line, which has no flag at all, was taken as header-only
// too and (this is not a header-only feed) never offered.
func TestDeepRevDnewslinkTrailingBlank(t *testing.T) {
	s := NewSite(t, SiteOptions{NoStart: true})
	dnlSpool(t, s)

	var (
		mu      sync.Mutex
		offered = map[string]bool{}
	)
	srv := fakeserver.New(t, fakeserver.Options{Greeting: "200 fake", Handler: fakeserver.LineHandler(
		func(_ *fakeserver.Conn, line string) ([]string, bool) {
			f := strings.Fields(line)
			if len(f) == 0 {
				return nil, false
			}
			switch strings.ToLower(f[0]) {
			case "mode":
				return []string{"203 stream ok"}, false
			case "quit":
				return []string{"205 bye"}, true
			case "check":
				if len(f) < 2 {
					return nil, false
				}
				mu.Lock()
				defer mu.Unlock()
				offered[f[1]] = true
				return []string{"438 " + f[1]}, false
			}
			return nil, false
		})})

	const plain = "<blank@e2e.test>"
	q := "D.00000001/B.0001 <" + strings.Repeat("h", 60) + "@e2e.test> 0,100 H\n" +
		"D.00000001/B.0001 " + plain + " 100,100 \n"
	if err := os.WriteFile(filepath.Join(s.Dir, "dqueue", "out.S00000"), []byte(q), 0o644); err != nil {
		t.Fatal(err)
	}

	out, err := s.TryRun("dnewslink", "-N", "1", "-S", "0", "-b", "out.S%05d", "-h", "127.0.0.1",
		"-P", strconv.Itoa(srv.Port()), "-t", "5")
	t.Logf("dnewslink: err=%v\n%s", err, out)

	mu.Lock()
	defer mu.Unlock()
	if srv.Accepted() < 1 {
		t.Fatalf("dnewslink did not connect")
	}
	if !offered[plain] {
		t.Errorf("the queue line that ends in a blank was not offered (offered: %v); queue file now:\n%s",
			offered, dnlQueue(t, s, "out.S00000"))
	}
}

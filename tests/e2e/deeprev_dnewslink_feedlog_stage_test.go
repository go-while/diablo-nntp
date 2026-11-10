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

// TestDeepRevDnewslinkFeedlogStage: dnewslink -A writes one line per article to
// feedlog.<host>: message-id, result, stage, reason. The stage says at which step the peer
// answered. For a streaming feed it was "-" on every line: the field starts as "-" and the
// streaming code set it only when it was empty. A refusal and a deferral at CHECK now say
// "stream", and so does the answer to the second offer of the deferred article. (The
// "resend-" prefix is set while dnewslink is still working through its defer queue; with
// the single deferred article here that is over when the answer comes.)
func TestDeepRevDnewslinkFeedlogStage(t *testing.T) {
	s := NewSite(t, SiteOptions{NoStart: true})
	dnlSpool(t, s)

	const refused, deferred = "<refused@e2e.test>", "<deferred@e2e.test>"
	var (
		mu     sync.Mutex
		offers int // CHECKs for the deferred article
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
				if len(f) > 1 && f[1] == deferred {
					mu.Lock()
					offers++
					first := offers == 1
					mu.Unlock()
					if first {
						return []string{"431 " + f[1]}, false
					}
				}
				if len(f) > 1 {
					return []string{"438 " + f[1]}, false
				}
			}
			return nil, false
		})})

	q := "D.00000001/B.0001 " + refused + " 0,100\n" +
		"D.00000001/B.0001 " + deferred + " 100,100\n"
	if err := os.WriteFile(filepath.Join(s.Dir, "dqueue", "out.S00000"), []byte(q), 0o644); err != nil {
		t.Fatal(err)
	}

	out, err := s.TryRun("dnewslink", "-Aall", "-N", "1", "-S", "0", "-b", "out.S%05d", "-h", "127.0.0.1",
		"-P", strconv.Itoa(srv.Port()), "-t", "5")
	t.Logf("dnewslink: err=%v\n%s", err, out)
	if srv.Accepted() < 1 {
		t.Fatalf("dnewslink did not connect")
	}

	log := s.ReadFile(t, "log/feedlog.127.0.0.1")
	for _, want := range []string{
		refused + " refused stream ",
		deferred + " deferred stream ",
		deferred + " refused stream ",
	} {
		if !strings.Contains(log, want) {
			t.Errorf("feedlog has no line with %q:\n%s", want, log)
		}
	}
}

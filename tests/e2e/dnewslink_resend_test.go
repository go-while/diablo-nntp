package e2e

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/go-while/nntp-testkit/fakeserver"
)

func TestDnewslinkFresh431DuringReplay(t *testing.T) {
	for _, size := range []int{1022, 1024} {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			testDnewslinkFresh431DuringReplay(t, size)
		})
	}
}

func testDnewslinkFresh431DuringReplay(t *testing.T, size int) {
	s := NewSite(t, SiteOptions{NoStart: true, Config: map[string]string{"feederrtstats": "label"}})
	dnlSpool(t, s)
	var mu sync.Mutex
	seen := make(map[string]int)
	const target = "<replay-1@e2e.test>"
	pending := false
	replay := false
	targetTakes := 0
	srv := fakeserver.New(t, fakeserver.Options{Greeting: "200 fake", Handler: fakeserver.LineHandler(func(_ *fakeserver.Conn, line string) ([]string, bool) {
		f := strings.Fields(strings.ToLower(line))
		if len(f) == 0 {
			return nil, false
		}
		mu.Lock()
		defer mu.Unlock()
		switch f[0] {
		case "mode":
			return []string{"203 stream ok"}, false
		case "check":
			seen[f[1]]++
			var n int
			fmt.Sscanf(f[1], "<replay-%d@e2e.test>", &n)
			if n >= 2 && n <= 81 && seen[f[1]] == 1 {
				return []string{"431 " + f[1] + " later"}, false
			}
			if n >= 2 && n <= 81 && seen[f[1]] == 2 && pending && !replay {
				replay = true
				return []string{"431 " + target + " later", "238 " + f[1]}, false
			}
			return []string{"238 " + f[1]}, false
		case "takethis":
			if f[1] == target {
				targetTakes++
			}
			if f[1] == target && !pending {
				pending = true
				return nil, false
			}
			return []string{"239 " + f[1]}, false
		case "quit":
			return []string{"205 bye"}, true
		}
		return nil, false
	})})
	targetLine := "D.00000001/B.0001 " + target + " 100,100"
	targetLine += strings.Repeat(" ", size-len(targetLine)-1) + "\n"
	var q strings.Builder
	for i := 1; i <= 120; i++ {
		if i == 1 {
			q.WriteString(targetLine)
		} else {
			fmt.Fprintf(&q, "D.00000001/B.0001 <replay-%d@e2e.test> %d,100\n", i, i*100)
		}
	}
	if err := os.WriteFile(filepath.Join(s.Dir, "dqueue", "out.S00000"), []byte(q.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := s.TryRun("dnewslink", "-Aall", "-N", "1", "-S", "0", "-b", "out.S%05d", "-h", "127.0.0.1",
		"-P", fmt.Sprint(srv.Port()), "-t", "5", "-w", "1", "-M", "20")
	t.Logf("dnewslink: err=%v\n%s", err, out)
	if err != nil {
		t.Fatalf("dnewslink failed: %v", err)
	}
	mu.Lock()
	sawReplay := replay
	nTarget := targetTakes
	mu.Unlock()
	if !sawReplay {
		t.Fatal("peer did not answer the pending fresh TAKETHIS during replay")
	}
	if nTarget != 1 {
		t.Errorf("target TAKETHIS sent %d times, want 1", nTarget)
	}
	if got := dnlQueue(t, s, "out.S00000"); got != targetLine {
		t.Errorf("fresh 431 queue line differs: got %q, want %q", got, targetLine)
	}
	if a, q := srv.Accepted(), srv.Count("QUIT"); a != 1 || q != 1 {
		t.Errorf("connections=%d QUIT=%d, want 1 each", a, q)
	}
	if log := rcuFeedlog(s); !strings.Contains(log, target+" kept") {
		t.Errorf("fresh 431 article was not logged as kept: %s", log)
	}
	stats := s.Run(t, "dfeedinfo", "-o", "-r", "out")
	if !hasDnewslinkStatsField(stats, "rej=0") {
		t.Errorf("fresh 431 counted as a rejection: %s", stats)
	}
}

func hasDnewslinkStatsField(out, want string) bool {
	for _, field := range strings.Fields(out) {
		if field == want {
			return true
		}
	}
	return false
}

func TestDnewslinkTakethis431RejectedBytes(t *testing.T) {
	const id = "<resend-bytes@e2e.test>"
	s := NewSite(t, SiteOptions{NoStart: true, Config: map[string]string{"feederrtstats": "label"}})
	dnlSpool(t, s)
	var takes atomic.Int32
	srv := fakeserver.New(t, fakeserver.Options{Greeting: "200 fake", Handler: fakeserver.LineHandler(func(c *fakeserver.Conn, line string) ([]string, bool) {
		switch {
		case strings.HasPrefix(strings.ToLower(line), "mode stream"):
			return []string{"203 stream ok"}, false
		case strings.HasPrefix(strings.ToLower(line), "check "):
			return []string{"238 " + id}, false
		case strings.HasPrefix(strings.ToLower(line), "takethis "):
			for {
				part, err := c.ReadLine()
				if err != nil {
					t.Errorf("reading article block: %v", err)
					return nil, true
				}
				if part == "." {
					break
				}
			}
			takes.Add(1)
			return []string{"431 " + id + " later"}, false
		case strings.HasPrefix(strings.ToLower(line), "quit"):
			return []string{"205 bye"}, true
		}
		return nil, false
	})})
	line := "D.00000001/B.0001 " + id + " 0,100\n"
	if err := os.WriteFile(filepath.Join(s.Dir, "dqueue", "out.S00000"), []byte(line), 0o644); err != nil {
		t.Fatal(err)
	}
	runOut, err := s.TryRun("dnewslink", "-Aall", "-N", "1", "-S", "0", "-b", "out.S%05d", "-h", "127.0.0.1",
		"-P", fmt.Sprint(srv.Port()), "-t", "5", "-w", "1")
	if err != nil {
		t.Fatalf("dnewslink failed: %v\n%s", err, runOut)
	}
	if got := takes.Load(); got != 2 {
		t.Fatalf("TAKETHIS sent %d times, want 2", got)
	}
	out := s.Run(t, "dfeedinfo", "-o", "-r", "out")
	if !hasDnewslinkStatsField(out, "rej=1") || !hasDnewslinkStatsField(out, "rejbytes=100") {
		t.Errorf("431 reject statistics: want rej=1 and rejbytes=100; got %s", out)
	}
	rcuSent(t, s, srv, id)
}

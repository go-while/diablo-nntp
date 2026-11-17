package e2e

import (
	"errors"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// noArgDirectives are dnewsfeeds options that take a number. A line holding only the name
// (a half-typed edit, say) must be reported as a configuration error, not crash the
// program that loads the file.
var noArgDirectives = []string{
	"feedertxbuf", "feederrxbuf", "startdelay", "delayfeed", "delayinfeed",
	"transmitbuf", "receivebuf", "setqos", "settos", "maxparallel", "maxqueue",
	"compress", "maxstream", "queueskip", "priority", "port",
}

// TestDnewsfeedsDirectiveWithoutArgument: dfeedtest loads dnewsfeeds the way diablo does.
// Fails on main: it died with SIGSEGV for each of these directives.
func TestDnewsfeedsDirectiveWithoutArgument(t *testing.T) {
	for _, d := range noArgDirectives {
		t.Run(d, func(t *testing.T) {
			s := NewSite(t, SiteOptions{NoStart: true, Newsfeeds: GlobalLabel + `label peer
    inhost ` + PeerIP + `
    addgroup *
    ` + d + `
end
`})
			out, err := s.TryRun("dfeedtest", "-n", "e2e.a", "-s", "100", "-p", "x")
			var ee *exec.ExitError
			if errors.As(err, &ee) && ee.ExitCode() < 0 {
				t.Errorf("dfeedtest was killed (%v) by %q without an argument:\n%s", err, d, out)
			}
		})
	}
}

// TestDnewsfeedsReloadWithoutArgument: diablo rereads dnewsfeeds about once a minute while it
// runs, so saving a half-typed line took the master down. Fails on main: the master exits
// within the next reload.
// The master looks at dnewsfeeds at its first pass in a new minute (at least every 5 s, its
// select() timeout), and a file younger than 10 s is left alone for that minute: its reload
// comes within about 5 s after the first minute boundary that is 11 s or more behind the
// write, up to a minute and a quarter after it. The test waits until the master has logged
// the line without an argument (only the master loads the file then: the first peer's process
// has exited), so it knows the reload happened.
func TestDnewsfeedsReloadWithoutArgument(t *testing.T) {
	s := NewSite(t, SiteOptions{})
	p := s.Peer(t)
	p.ExpectCmd(205, "QUIT")
	time.Sleep(3 * time.Second)
	s.WriteFile(t, "dnewsfeeds", GlobalLabel+"label peer\n    inhost "+PeerIP+"\n    addgroup *\n    maxparallel\nend\n")
	written := time.Now()
	boundary := written.Truncate(time.Minute).Add(time.Minute)
	if boundary.Sub(written) < 11*time.Second {
		boundary = boundary.Add(time.Minute)
	}
	// 5 s select() timeout, and room for a busy host
	deadline := boundary.Add(20 * time.Second)
	const logged = "command in unexpected position or command requires argument"
	for !strings.Contains(generalLog(s), logged) {
		if time.Now().After(deadline) {
			t.Fatalf("the diablo master did not reload dnewsfeeds within 20 s after the minute boundary %s (written %s): no %q in the log:\n%s",
				boundary.Format("15:04:05"), written.Format("15:04:05.000"), logged, s.logOutput())
		}
		select {
		case <-s.Proc.Done():
			t.Fatalf("the diablo master exited after dnewsfeeds got a directive without an argument:\n%s", s.logOutput())
		case <-time.After(200 * time.Millisecond):
		}
	}
	// and it still serves
	p = s.Peer(t)
	p.ExpectCmd(205, "QUIT")
	select {
	case <-s.Proc.Done():
		t.Errorf("the diablo master exited after the reload:\n%s", s.logOutput())
	default:
	}
}

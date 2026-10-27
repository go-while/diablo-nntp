package e2e

import (
	"errors"
	"os/exec"
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
func TestDnewsfeedsReloadWithoutArgument(t *testing.T) {
	s := NewSite(t, SiteOptions{})
	p := s.Peer(t)
	expect(t, p, 205, "QUIT")
	time.Sleep(3 * time.Second)
	s.WriteFile(t, "dnewsfeeds", GlobalLabel+"label peer\n    inhost "+PeerIP+"\n    addgroup *\n    maxparallel\nend\n")
	select {
	case <-s.Proc.Done():
		t.Errorf("the diablo master exited after dnewsfeeds got a directive without an argument")
	case <-time.After(30 * time.Second):
	}
	// and it still serves
	p = s.Peer(t)
	expect(t, p, 205, "QUIT")
}

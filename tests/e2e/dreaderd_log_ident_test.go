package e2e

import (
	"strings"
	"testing"
	"time"

	testkit "github.com/go-while/nntp-testkit"
)

// dreaderd logged its "Listening on ..." lines (and the warnings before them) into the general
// log without "dreaderd:" in front, because the log identity was set only after the listen
// sockets were opened; later lines had it ("2026-11-15 17:17:28.701  Listening on ALL:119" next
// to "... dreaderd: Installing access cache").

// TestReaderListenLineIdent: with a listen socket of its own (no inherited one), every "Listening
// on" line in the general log carries the "dreaderd:" identity. Fails on main: the line has none.
func TestReaderListenLineIdent(t *testing.T) {
	_, spool := spoolStore(t)
	s := NewReaderSite(t, ReaderOptions{
		Spool:   spool,
		Config:  map[string]string{"path_generallog": "%s/general.log"},
		NoStart: true,
	})
	startReaderArgs(t, s)
	var log string
	testkit.WaitFor(t, 10*time.Second, "the listen line in the general log", func() bool {
		log = generalLog(s)
		return strings.Contains(log, "Listening on")
	})
	n := 0
	for _, l := range strings.Split(log, "\n") {
		if !strings.Contains(l, "Listening on") {
			continue
		}
		n++
		// "2006-01-02 15:04:05.000 dreaderd: Listening on ..."
		if !strings.Contains(l, " dreaderd: Listening on ") {
			t.Errorf("listen line without the dreaderd identity: %q", l)
		}
	}
	if n == 0 {
		t.Errorf("no listen line in the general log:\n%s", log)
	}
}

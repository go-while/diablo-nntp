package e2e

import (
	"strings"
	"testing"
	"time"

	testkit "github.com/go-while/nntp-testkit"
)

// D-203: with readercanlock none dreaderd refused every cancel, but only after it had scanned
// the overview of the groups the cancel names for the target. A cancel whose target was not
// there was logged as "target not here yet, dropped (readercanlock)". It is now refused before
// any lookup, with the same log line for every cancel and supersede.

// TestCancelLockNoneNoLookup: a cancel of an article that is there, a cancel of one that is not
// and a supersede are all logged "refused (readercanlock none)", and nothing is removed. Fails on
// main: the cancel of the missing article logs "target not here yet, dropped (readercanlock)".
func TestCancelLockNoneNoLookup(t *testing.T) {
	skipWithoutCanLockMode(t)
	st, spool := spoolStore(t)
	s := NewReaderSite(t, ReaderOptions{
		Spool:   spool,
		Config:  map[string]string{"readercanlock": "none", "path_generallog": "%s/general.log"},
		Files:   map[string]string{"dcontrol.ctl": "cancel:*:*:doit,break\n"},
		NoStart: true,
	})
	_, port, _ := strings.Cut(s.Addr, ":")
	startReader(t, s, "[127.0.0.1]:"+port)

	target := msgID(t)
	superseded := msgID(t)
	missing := msgID(t)
	c := s.Reader(t)
	defer c.ExpectCmd(205, "QUIT")
	feedAndSettle(t, s, st, c,
		newArticle(target, "e2e.test", "target", "the target"),
		newArticle(superseded, "e2e.test", "superseded", "the superseded article"),
		cancelArticle(t, target, "e2e.test"),
		cancelArticle(t, missing, "e2e.test"),
		withHeaders(newArticle(msgID(t), "e2e.test", "supersedes", "the new version"), "Supersedes: "+superseded),
	)

	var log string
	testkit.WaitFor(t, 5*time.Second, "the three cancels in the general log", func() bool {
		log = generalLog(s)
		return strings.Count(log, "refused (readercanlock none)") >= 3 || strings.Contains(log, "target not here yet")
	})
	for _, id := range []string{target, missing, superseded} {
		if want := "cancel " + id + ": refused (readercanlock none), 0 item(s)"; !strings.Contains(log, want) {
			t.Errorf("general log lacks %q", want)
		}
	}
	if strings.Contains(log, "target not here yet") {
		t.Errorf("a cancel was looked up although readercanlock none refuses it:\n%s", log)
	}
	for _, id := range []string{target, superseded} {
		if code, _ := c.Cmd("STAT %s", id); code != 223 {
			t.Errorf("STAT %s: %d, want 223 (readercanlock none removes nothing)", id, code)
		}
	}
	if t.Failed() {
		t.Logf("general log:\n%s", log)
	}
}

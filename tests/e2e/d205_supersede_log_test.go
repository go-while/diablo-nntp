package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	testkit "github.com/go-while/nntp-testkit"
)

// D-205: dreaderd runs a Supersedes header as a made-up cancel of the old article, and logged
// the decision as "cancel <old id>: ...", the same line as for a cancel control message, with
// ">> control cancel" in the dcontrol.ctl log. A supersede is now logged as
// "supersede <old id> by <new id>: ..." and ">> control supersede"; a cancel is logged as before.

// TestSupersedeLogLine: under the default readercanlock require-auth, a supersede of an article
// without Cancel-Lock leaves the old article in place, stores the new one and logs
// "supersede <old> by <new>: ...". A cancel of another article without Cancel-Lock still logs
// "cancel <id>: ...". Fails on main: the supersede is logged as "cancel <old>: ...".
func TestSupersedeLogLine(t *testing.T) {
	skipWithoutCanLockMode(t)
	st, spool := spoolStore(t)
	s := NewReaderSite(t, ReaderOptions{
		Spool:   spool,
		Config:  map[string]string{"path_generallog": "%s/general.log"},
		Files:   map[string]string{"dcontrol.ctl": "cancel:*:*:doit,log=cancel.log\n"},
		NoStart: true,
	})
	_, port, _ := strings.Cut(s.Addr, ":")
	startReader(t, s, "[127.0.0.1]:"+port)

	old, replacement, target := msgID(t), msgID(t), msgID(t)
	c := s.Reader(t)
	defer c.ExpectCmd(205, "QUIT")
	feedAndSettle(t, s, st, c,
		newArticle(old, "e2e.test", "old version", "the old version"),
		newArticle(target, "e2e.test", "cancel target", "the cancel target"),
		withHeaders(newArticle(replacement, "e2e.test", "new version", "the new version"), "Supersedes: "+old),
		cancelArticle(t, target, "e2e.test"),
	)

	supersede := "supersede " + old + " by " + replacement + ": "
	cancel := "cancel " + target + ": "
	var log string
	testkit.WaitFor(t, 5*time.Second, "the supersede and the cancel in the general log", func() bool {
		log = generalLog(s)
		return strings.Contains(log, cancel) && (strings.Contains(log, supersede) || strings.Contains(log, "cancel "+old+": "))
	})
	if !strings.Contains(log, supersede) {
		t.Errorf("general log lacks %q", supersede)
	}
	if strings.Contains(log, "cancel "+old+": ") {
		t.Errorf("the supersede was logged as a cancel of %s", old)
	}
	if !strings.Contains(log, cancel) {
		t.Errorf("general log lacks %q", cancel)
	}
	// require-auth: neither old article has a Cancel-Lock, so both stay; the new one is stored
	for _, id := range []string{old, target, replacement} {
		if code, _ := c.Cmd("STAT %s", id); code != 223 {
			t.Errorf("STAT %s: %d, want 223", id, code)
		}
	}
	b, _ := os.ReadFile(filepath.Join(s.Dir, "log", "cancel.log"))
	if cl := string(b); !strings.Contains(cl, ">> control supersede, ") || !strings.Contains(cl, ">> control cancel, ") {
		t.Errorf("dcontrol.ctl log lacks \">> control supersede\" or \">> control cancel\":\n%s", cl)
	}
	if t.Failed() {
		t.Logf("general log:\n%s", log)
	}
}

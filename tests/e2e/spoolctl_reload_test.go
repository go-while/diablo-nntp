package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	testkit "github.com/go-while/nntp-testkit"
	"github.com/go-while/nntp-testkit/client"
)

// TestSpoolCtlBadReloadKeepsRunning (D-175): diablo reloads dspool.ctl when it changes (at
// most once a minute). A reload with an error in the new file keeps the old spool
// configuration and logs why; diablo goes on storing articles, and the next good file is
// loaded (here it stores into spool 01). Before, any error in loadSpoolCtl() was exit(1), so
// a typo saved while diablo ran stopped the feeder.
func TestSpoolCtlBadReloadKeepsRunning(t *testing.T) {
	if testing.Short() {
		t.Skip("waits for diablo's once-a-minute dspool.ctl check")
	}
	s := NewSite(t, SiteOptions{Config: map[string]string{"path_generallog": "%s/general.log"}})
	ctl := filepath.Join(s.Dir, "dspool.ctl")
	old, err := os.ReadFile(ctl)
	if err != nil {
		t.Fatal(err)
	}
	bad := string(old) + "\nspooool 01\n    path P.01\nend\n"
	if err := os.WriteFile(ctl, []byte(bad), 0o644); err != nil {
		t.Fatal(err)
	}
	future := time.Now().Add(2 * time.Second)
	_ = os.Chtimes(ctl, future, future) // a new mtime even within the same second

	testkit.WaitFor(t, 75*time.Second, "diablo to refuse the bad dspool.ctl", func() bool {
		return strings.Contains(generalLog(s), "keeping the old spool configuration")
	})
	select {
	case <-s.Proc.Done():
		t.Fatalf("diablo exited after the bad dspool.ctl\n%s", generalLog(s))
	default:
	}

	id := msgID(t)
	p := s.Peer(t)
	p.ExpectCmd(335, "IHAVE %s", id)
	p.MustSend(client.Stuff(newArticle(id, "e2e.test", "after a bad reload", "body")))
	p.ExpectStatus(235, "IHAVE after a bad reload")
	p.ExpectCmd(223, "STAT %s", id)

	// the fixed file is taken at a later check: new articles go to spool 01
	p01 := filepath.Join(s.Dir, "spool", "news", "P.01")
	if err := os.MkdirAll(p01, 0o755); err != nil {
		t.Fatal(err)
	}
	good := "spool 00\n    minfree 1m\nend\nspool 01\n    minfree 1m\nend\nmetaspool trad\n    spool 01\nend\nexpire * trad\n"
	if err := os.WriteFile(ctl, []byte(good), 0o644); err != nil {
		t.Fatal(err)
	}
	future = time.Now().Add(4 * time.Second)
	_ = os.Chtimes(ctl, future, future)
	p.ExpectCmd(205, "QUIT")
	deadline := time.Now().Add(75 * time.Second)
	for {
		// a new connection each time: its child forks from the master with the master's
		// configuration of that moment
		id := msgID(t)
		p = s.Peer(t)
		p.ExpectCmd(335, "IHAVE %s", id)
		p.MustSend(client.Stuff(newArticle(id, "e2e.test", "after the fixed dspool.ctl", "body")))
		p.ExpectStatus(235, "IHAVE after the fixed dspool.ctl")
		p.ExpectCmd(205, "QUIT")
		if d, _ := filepath.Glob(filepath.Join(p01, "D.*")); len(d) > 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("the fixed dspool.ctl was not loaded: nothing stored in spool 01\n%s", generalLog(s))
		}
		time.Sleep(2 * time.Second)
	}
}

// TestSpoolCtlBadReloadRetried (review note on #308): a dspool.ctl reload that fails for a
// reason that passes without a change to the file (here a spool partition that is not there
// yet, as when it is not mounted) is tried again a minute later, and taken once the partition
// is there. Before, the refused file was not looked at again until its mtime changed, and
// diablo kept the old spool set without saying so again.
func TestSpoolCtlBadReloadRetried(t *testing.T) {
	if testing.Short() {
		t.Skip("waits for two of diablo's once-a-minute dspool.ctl checks")
	}
	s := NewSite(t, SiteOptions{Config: map[string]string{"path_generallog": "%s/general.log"}})
	ctl := filepath.Join(s.Dir, "dspool.ctl")
	p01 := filepath.Join(s.Dir, "spool", "news", "P.01")
	good := "spool 00\n    minfree 1m\nend\nspool 01\n    minfree 1m\nend\nmetaspool trad\n    spool 01\nend\nexpire * trad\n"
	if err := os.WriteFile(ctl, []byte(good), 0o644); err != nil {
		t.Fatal(err)
	}
	future := time.Now().Add(2 * time.Second)
	_ = os.Chtimes(ctl, future, future)
	testkit.WaitFor(t, 75*time.Second, "diablo to refuse dspool.ctl without P.01", func() bool {
		return strings.Contains(generalLog(s), "next try in 1 min")
	})
	if err := os.MkdirAll(p01, 0o755); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(100 * time.Second)
	for {
		// a new connection each time: its child has the master's configuration of that moment
		id := msgID(t)
		p := s.Peer(t)
		p.ExpectCmd(335, "IHAVE %s", id)
		p.MustSend(client.Stuff(newArticle(id, "e2e.test", "after P.01 appeared", "body")))
		p.ExpectStatus(235, "IHAVE after P.01 appeared")
		p.ExpectCmd(205, "QUIT")
		if d, _ := filepath.Glob(filepath.Join(p01, "D.*")); len(d) > 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("dspool.ctl was not tried again: nothing stored in spool 01\n%s", generalLog(s))
		}
		time.Sleep(2 * time.Second)
	}
	select {
	case <-s.Proc.Done():
		t.Fatalf("diablo exited\n%s", generalLog(s))
	default:
	}
}

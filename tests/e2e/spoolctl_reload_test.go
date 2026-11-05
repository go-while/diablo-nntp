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
// configuration and logs why; diablo goes on storing articles. Before, any error in
// loadSpoolCtl() was exit(1), so a typo saved while diablo ran stopped the feeder.
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
	p.ExpectCmd(205, "QUIT")
}

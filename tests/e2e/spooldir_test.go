package e2e

import (
	"github.com/go-while/nntp-testkit/client"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestSpoolDirRemovedWhileWriting: when the spool directory a connection is writing into is
// removed (as dexpire does when it expires down to the newest directory), the next article on
// the same connection is still stored: diablo recreates the directory and changes into it
// again. Before, it kept retrying inside the removed directory and rejected the article with
// "IOError/Missing-dir" (437).
//
// diablo logs to a file in the site (path_generallog): on illumos, syslog has no LOG_PERROR,
// so in -d mode the log lines never reach stderr.
func TestSpoolDirRemovedWhileWriting(t *testing.T) {
	s := NewSite(t, SiteOptions{
		Config: map[string]string{"path_generallog": "%s/general.log"},
	})
	p := s.Peer(t)
	first := msgID(t)
	p.ExpectCmd(335, "IHAVE %s", first)
	p.MustSend(client.Stuff(newArticle(first, "e2e.test", "first", "body")))
	p.ExpectStatus(235, "IHAVE first")

	dirs, _ := filepath.Glob(filepath.Join(s.Dir, "spool", "news", "D.*"))
	if len(dirs) == 0 {
		t.Fatal("no spool directory after the first article")
	}
	for _, d := range dirs {
		if err := os.RemoveAll(d); err != nil {
			t.Fatal(err)
		}
	}

	second := msgID(t)
	p.ExpectCmd(335, "IHAVE %s", second)
	p.MustSend(client.Stuff(newArticle(second, "e2e.test", "second", "body")))
	p.ExpectStatus(235, "IHAVE after the spool directory was removed")
	p.ExpectCmd(223, "STAT %s", second)
	p.ExpectCmd(205, "QUIT")

	if again, _ := filepath.Glob(filepath.Join(s.Dir, "spool", "news", "D.*")); len(again) == 0 {
		t.Errorf("spool directory not recreated")
	}
	logged, _ := os.ReadFile(filepath.Join(s.Dir, "log", "general.log"))
	e := string(logged) + s.Proc.Stderr()
	if !strings.Contains(e, "was missing, created it") && !strings.Contains(e, "was removed while open") {
		t.Errorf("no log line about the recreated spool directory in log/general.log or stderr:\n%s", logged)
	}
}

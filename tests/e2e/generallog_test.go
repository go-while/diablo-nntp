package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestGeneralLogNotWritable: with path_generallog pointing at a file diablo cannot write,
// diablo reports the failure, logs to syslog instead, and keeps accepting articles. Before
// 6-CUR-20261024-BWH the failed open was itself logged through the general log, which
// retried the open until the process crashed with SIGSEGV.
func TestGeneralLogNotWritable(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root: root can write a read-only file")
	}
	s := NewSite(t, SiteOptions{
		Config: map[string]string{"path_generallog": "%s/general.log"},
		Files:  map[string]string{"log/general.log": ""},
		Setup: func(tb testing.TB, s *Site) {
			if err := os.Chmod(filepath.Join(s.Dir, "log", "general.log"), 0o444); err != nil {
				tb.Fatal(err)
			}
		},
	})

	p := s.Peer(t)
	if !strings.HasPrefix(p.Greeting, "200 ") {
		t.Fatalf("peer greeting %q, want 200", p.Greeting)
	}
	id := msgID(t)
	expect(t, p, 335, "IHAVE %s", id)
	send(t, p, wire(newArticle(id, "e2e.test", "general log not writable", "body")))
	status(t, p, 235, "IHAVE article")
	expect(t, p, 205, "QUIT")

	if !strings.Contains(s.Proc.Stderr(), "Unable to open log file") {
		t.Errorf("diablo did not report the unwritable general log; output:\n%s", s.Proc.Stderr())
	}
}

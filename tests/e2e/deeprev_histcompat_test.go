package e2e

import (
	"net"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	testkit "github.com/go-while/nntp-testkit"
	"github.com/go-while/nntp-testkit/proc"
)

// TestDeepRevHistoryCompatHashLookup (NEW-1): "hash crc/prime" (the setting
// samples/diablo.config recommends when moving from the old hash method) makes
// HistoryLookup() try the second method when the first finds nothing. The retry is a
// recursive call that fills the caller's record, and the outer call then copied its own
// local record over it: the last record of the chain it had walked with the first method (some
// other article), or zeros when that chain was empty. The message-id was "found", but with
// another article's file, offset and size: STAT, HEAD, BODY and ARTICLE on the feeder (which
// is how a reader fetches articles) answered "not found" or with a different article.
//
// A site stores three articles with "hash prime", then runs with "hash crc/prime": IHAVE
// still knows them (435), and STAT and HEAD must give the article.
func TestDeepRevHistoryCompatHashLookup(t *testing.T) {
	s := NewSite(t, SiteOptions{Config: map[string]string{"hash": "prime"}})
	ids := s.Inject(t, "e2e.test", 3)

	// Stop diablo, switch the hash method, start it again on the same history and spool.
	s.Dicmd(t, "exit")
	select {
	case <-s.Proc.Done():
	case <-time.After(20 * time.Second):
		t.Fatal("diablo still running 20 s after dicmd exit")
	}
	conf := s.ReadFile(t, "diablo.config")
	if !strings.Contains(conf, "hash\tprime\n") {
		t.Fatalf("diablo.config has no \"hash prime\" line:\n%s", conf)
	}
	s.WriteFile(t, "diablo.config", strings.Replace(conf, "hash\tprime\n", "hash\tcrc/prime\n", 1))
	s.Addr = net.JoinHostPort("127.0.0.1", strconv.Itoa(testkit.FreePort(t)))
	s.Proc = startSiteTCP(t, proc.Options{
		Path: filepath.Join(s.Bin, "diablo"),
		Args: []string{"-d", "-p", PathHost, "-h", PathHost, "-P", s.Addr, "server"},
		Env:  slices.Clone(s.Env),
		Dir:  s.Dir,
	}, s.Addr)
	t.Cleanup(func() { s.shutdown(t) })

	p := s.Peer(t)
	for _, id := range ids {
		// found through the second method: a duplicate
		p.ExpectCmd(435, "IHAVE %s", id)
		if code, line := p.Cmd("STAT %s", id); code != 223 {
			t.Errorf("STAT %s: %q, want 223 (IHAVE says the article is known)", id, line)
		}
		code, line := p.Cmd("HEAD %s", id)
		if code != 221 {
			t.Errorf("HEAD %s: %q, want 221", id, line)
			continue
		}
		if head := p.ReadDotLines(); !containsLine(head, "Message-ID: "+id) {
			t.Errorf("HEAD %s returned another article: %q", id, head)
		}
	}
	p.ExpectCmd(205, "QUIT")
}

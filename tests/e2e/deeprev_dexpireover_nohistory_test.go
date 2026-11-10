package e2e

import (
	"os"
	"strings"
	"testing"

	testkit "github.com/go-while/nntp-testkit"
)

// TestDeepRevDexpireoverNoHistory (L7-05): "dexpireover -e" expires the overview entries
// whose article the local history does not know or has marked expired. It opened the history
// read-write with O_CREAT, so on a host without a history at the configured path (a reader
// without a local spool, a wrong -C, a moved file) it created an empty one, found no article
// in it, and expired every overview entry of every group.
//
// On a reader site without a dhistory, dexpireover -e must leave the overview alone (and not
// create a history).
func TestDeepRevDexpireoverNoHistory(t *testing.T) {
	st, spool := spoolStore(t)
	s := NewReaderSite(t, ReaderOptions{Spool: spool, Config: map[string]string{"hsize": "256k"}})
	id1, id2 := msgID(t), msgID(t)
	feedReader(t, s, st, "e2e.test",
		newArticle(id1, "e2e.test", "overview one", "body"),
		newArticle(id2, "e2e.test", "overview two", "body"))

	c := s.Reader(t)
	testkit.WaitFor(t, testkit.Deadline(), "two articles in GROUP e2e.test", func() bool {
		_, line := c.Cmd("GROUP e2e.test")
		return strings.HasPrefix(line, "211 2 1 2 ")
	})
	c.ExpectCmd(224, "OVER 1-2")
	if before := c.ReadDotLines(); len(before) != 2 {
		t.Fatalf("OVER 1-2 before dexpireover: %d lines, want 2: %q", len(before), before)
	}
	c.ExpectCmd(205, "QUIT")

	hist := strings.TrimSpace(s.Run(t, "dpath", "dhistory"))
	if _, err := os.Stat(hist); err == nil {
		t.Fatalf("the reader site has a history file %s: the test needs a site without one", hist)
	}

	out, err := s.TryRun("dexpireover", "-e", "-v")
	t.Logf("dexpireover -e -v: err=%v\n%s", err, out)

	// dreaderd reads the index through a shared map: a change shows at once.
	c2 := s.Reader(t)
	_, group := c2.Cmd("GROUP e2e.test")
	var after []string
	code, line := c2.Cmd("OVER 1-2")
	if code == 224 {
		after = c2.ReadDotLines()
	}
	if len(after) != 2 {
		t.Errorf("dexpireover -e without a history expired the overview: GROUP %q, OVER 1-2 %q with %d lines, want 2: %q",
			group, line, len(after), after)
	}
	c2.ExpectCmd(205, "QUIT")
	if _, err := os.Stat(hist); err == nil {
		t.Errorf("dexpireover -e created the history file %s", hist)
	}
}

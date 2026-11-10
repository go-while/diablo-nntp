package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	testkit "github.com/go-while/nntp-testkit"
)

// TestDeepRevEmptyGzDataFileLeaksDescriptor (L5-03): dreaderd unpacks an overview data file
// that dexpireover -z compressed into a temporary file (dreaderd/group.c, MakeOverHFile()).
// When the .gz cannot be mapped, which is the case for an empty one (what a full disk or an
// interrupted dexpireover -z can leave), the function gave up without closing the temporary
// file it had just made. The failure is not remembered, so every request that touches the
// data file lost one descriptor, until the reader process had none left.
//
// A group whose data file is replaced by an empty .gz: 60 OVER requests must not raise the
// reader's descriptor count.
func TestDeepRevEmptyGzDataFileLeaksDescriptor(t *testing.T) {
	skipWithoutZlib(t)
	requireProcessInspection(t, true)
	st, spool := spoolStore(t)
	s := NewReaderSite(t, ReaderOptions{Spool: spool})
	feedReader(t, s, st, "e2e.test",
		newArticle(msgID(t), "e2e.test", "data one", "body"),
		newArticle(msgID(t), "e2e.test", "data two", "body"))
	c := s.Reader(t)
	testkit.WaitFor(t, testkit.Deadline(), "two articles in GROUP e2e.test", func() bool {
		_, line := c.Cmd("GROUP e2e.test")
		return strings.HasPrefix(line, "211 2 1 2 ")
	})
	c.ExpectCmd(205, "QUIT")

	// Stop dreaderd (it holds the plain data file open), swap the data file for an empty
	// .gz, start a fresh one.
	s.Proc.Stop()
	m, _ := filepath.Glob(filepath.Join(s.Dir, "spool", "group", "*", "data.*"))
	m2, _ := filepath.Glob(filepath.Join(s.Dir, "spool", "group", "data.*"))
	data := append(m, m2...)
	if len(data) == 0 {
		t.Fatalf("no overview data file below %s/spool/group", s.Dir)
	}
	for _, f := range data {
		if err := os.Remove(f); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(f+".gz", nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	_, port, _ := strings.Cut(s.Addr, ":")
	startReader(t, s, "[127.0.0.1]:"+port)

	c = s.Reader(t)
	c.ExpectCmd(211, "GROUP e2e.test")
	over := func() {
		t.Helper()
		if code, _ := c.Cmd("OVER 1-2"); code == 224 {
			c.ReadDotLines()
		}
	}
	over() // the reader has the group open now
	count := func() map[int]int {
		m := map[int]int{}
		for _, pid := range sitePids(t, s, "dreaderd") {
			m[pid] = fdCount(t, pid)
		}
		return m
	}
	before := count()
	const requests = 60
	for range requests {
		over()
	}
	time.Sleep(200 * time.Millisecond)
	after := count()
	c.ExpectCmd(205, "QUIT")

	for pid, n := range before {
		if m, ok := after[pid]; ok && n >= 0 && m > n+5 {
			t.Errorf("dreaderd process %d: %d descriptors before and %d after %d OVER requests on a group with an empty .gz data file",
				pid, n, m, requests)
		}
	}
}

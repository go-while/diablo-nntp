package e2e

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-while/nntp-testkit/client"
)

// TestDeepRevDataFileFailureReleasesGroup (L5-04): to store the overview record of a fed
// article the reader takes a reference on the group's overview (GetOverInfo()) and opens
// the group's data file (dreaderd/group.c, WriteOverview()). When the data file could not
// be opened or created (no descriptors, disk full, permissions) the function answered 400
// and returned without giving the reference back. A referenced overview is never dropped
// from the cache of open groups (8 without a reference), so every group this happened to
// kept its index file open, mapped and locked for the life of the feed process.
//
// 30 groups whose data file is replaced by a directory, so that it cannot be opened: one
// article fed to each must not leave 30 more descriptors open in a dreaderd process.
func TestDeepRevDataFileFailureReleasesGroup(t *testing.T) {
	requireProcessInspection(t, true)
	const n = 30
	groups := []string{"e2e.test"}
	for i := range n {
		groups = append(groups, fmt.Sprintf("e2e.g%02d", i))
	}
	st, spool := spoolStore(t)
	s := NewReaderSite(t, ReaderOptions{Spool: spool, Groups: groups})
	for _, g := range groups[1:] {
		feedReader(t, s, st, "e2e.test", newArticle(msgID(t), g, "first in "+g, "body"))
	}

	// Stop dreaderd (it holds data files open), put a directory where each data file was,
	// start a fresh one.
	s.Proc.Stop()
	m, _ := filepath.Glob(filepath.Join(s.Dir, "spool", "group", "*", "data.*"))
	m2, _ := filepath.Glob(filepath.Join(s.Dir, "spool", "group", "data.*"))
	data := append(m, m2...)
	if len(data) < n {
		t.Fatalf("%d overview data files below %s/spool/group, want %d", len(data), s.Dir, n)
	}
	for _, f := range data {
		if err := os.Remove(f); err != nil {
			t.Fatal(err)
		}
		if err := os.Mkdir(f, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	_, port, _ := strings.Cut(s.Addr, ":")
	startReader(t, s, "[127.0.0.1]:"+port)

	count := func() map[int]int {
		m := map[int]int{}
		for _, pid := range sitePids(t, s, "dreaderd") {
			m[pid] = fdCount(t, pid)
		}
		return m
	}
	// one article that is stored (e2e.test has no data file yet): the feed process is up
	feedReader(t, s, st, "e2e.test", newArticle(msgID(t), "e2e.test", "warm up", "body"))
	before := count()

	refused := 0
	for _, g := range groups[1:] {
		id := msgID(t)
		p := s.dialFrom(t, PeerIP)
		p.ExpectCmd(335, "IHAVE %s", id)
		p.MustSend(client.Stuff(newArticle(id, g, "second in "+g, "body")))
		if line, _ := p.ReadLine(); strings.HasPrefix(line, "400") {
			refused++
		}
		p.Close()
	}
	if refused != n {
		t.Fatalf("%d of %d articles were answered 400: the data files could be opened, the test did not get its failure", refused, n)
	}
	time.Sleep(300 * time.Millisecond)
	after := count()

	for pid, b := range before {
		if a, ok := after[pid]; ok && b >= 0 && a > b+n/2 {
			t.Errorf("dreaderd process %d: %d descriptors before and %d after %d groups whose data file could not be opened: their overviews stay referenced and open",
				pid, b, a, n)
		}
	}
}

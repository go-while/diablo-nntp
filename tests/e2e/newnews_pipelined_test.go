package e2e

import (
	"fmt"
	"strings"
	"testing"
	"time"

	testkit "github.com/go-while/nntp-testkit"
)

// TestReaderNewnewsPipelinedCommand (D-249): a command the client sends right after NEWNEWS,
// in the same write, is answered after the final "." of the NEWNEWS list. dreaderd called
// NNCommand() at the end of each group of the list; when one group did not fit one pass of
// the reader loop (its message-ids fill the output buffer, 7 KB without BIG_MBUF, 62 KB with
// it, or it has more than 10,000 overview records), that call read the next command and
// answered it there, inside the 230 block, and the rest of the list followed. The client then
// took the reply as a line of the list and found no reply after the ".".
//
// e2e.test gets 400 articles with message-ids of about 190 bytes (some 76 KB: more than one
// pass also with BIG_MBUF, on the default path without USE_OVER_MADVISE; 400 is below the 512
// article slots a new group's overview index gets by default, dexpire.ctl "a", so no entry is
// reused) and e2e.other one. NEWNEWS for both groups, DATE and QUIT go in one write; the list
// must hold each of the 401 message-ids once and nothing else, then DATE gets its 111, then
// QUIT its 205.
func TestReaderNewnewsPipelinedCommand(t *testing.T) {
	const nBig = 400
	st, spool := spoolStore(t)
	access := strings.Replace(DefaultReaderAccess, "readerdef rp\n", "readerdef rp\n    allownewnews yes\n", 1)
	s := NewReaderSite(t, ReaderOptions{Spool: spool, Access: access})

	want := map[string]bool{}
	idBytes := 0
	arts := make([][]string, 0, nBig)
	for i := 0; i < nBig; i++ {
		id := msgID(t)
		id = "<" + strings.Repeat("n", 120) + "." + id[1:]
		want[id] = true
		idBytes += len(id) + 2
		arts = append(arts, newArticle(id, "e2e.test", fmt.Sprintf("newnews pipelined %d", i), "body"))
	}
	feedReader(t, s, st, "e2e.test", arts...)
	other := msgID(t)
	want[other] = true
	feedReader(t, s, st, "e2e.other", newArticle(other, "e2e.other", "newnews pipelined other", "body"))
	if idBytes <= 64*1024 {
		t.Fatalf("the message-ids of e2e.test are %d bytes, not more than one pass of output", idBytes)
	}

	c := s.Reader(t)
	testkit.WaitFor(t, 20*time.Second, "all articles in GROUP e2e.test and e2e.other", func() bool {
		_, l1 := c.Cmd("GROUP e2e.test")
		_, l2 := c.Cmd("GROUP e2e.other")
		return strings.HasPrefix(l1, fmt.Sprintf("211 %d ", nBig)) && strings.HasPrefix(l2, "211 1 ")
	})

	if err := c.Send("NEWNEWS e2e.test,e2e.other 000101 000000 GMT\r\nDATE\r\nQUIT\r\n"); err != nil {
		t.Fatalf("writing NEWNEWS, DATE and QUIT: %v", err)
	}
	code, line, err := c.ReadStatus()
	if err != nil || code != 230 {
		t.Fatalf("NEWNEWS: %q %v, want 230", line, err)
	}
	lines, err := c.TryReadDotLines()
	if err != nil {
		t.Fatalf("the NEWNEWS list: %v (after %d lines)", err, len(lines))
	}
	seen := map[string]bool{}
	for i, l := range lines {
		if !want[l] {
			t.Fatalf("line %d of %d of the NEWNEWS list is %q, not one of its message-ids: the pipelined DATE was answered inside the 230 block", i+1, len(lines), l)
		}
		if seen[l] {
			t.Fatalf("line %d of the NEWNEWS list: %s listed twice", i+1, l)
		}
		seen[l] = true
	}
	if len(seen) != len(want) {
		t.Fatalf("the NEWNEWS list has %d message-ids, want %d", len(seen), len(want))
	}
	if code, line, err := c.ReadStatus(); err != nil || code != 111 {
		t.Fatalf("DATE after the NEWNEWS list: %q %v, want 111", line, err)
	}
	if code, line, err := c.ReadStatus(); err != nil || code != 205 {
		t.Fatalf("QUIT: %q %v, want 205", line, err)
	}
}

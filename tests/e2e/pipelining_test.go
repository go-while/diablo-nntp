package e2e

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	testkit "github.com/go-while/nntp-testkit"
	"github.com/go-while/nntp-testkit/client"
)

// RFC 3977 section 3.5: a server MUST allow pipelining (a client sending the next command before the
// response to the previous one), MUST NOT throw away text received after a command, and MUST process the
// commands in the order they are sent. There is no capability for it: it is mandatory.

// pipeReply is one response: the status line and, for a multi-line response, its lines up to the dot
// (un-stuffed; the client is strict, so a line starting with a single dot is an error).
type pipeReply struct {
	code   int
	status string
	lines  []string
}

// pipeline sends all commands in one write and reads one response per entry of multi (true: a
// multi-line response). The last answered command must be QUIT: nothing may follow its response, and
// anything sent after it is not answered.
//
// The write runs in its own goroutine while the responses are read. A client that wrote everything
// before reading could block for good: dreaderd stops reading commands while its replies cannot be sent,
// so once the replies fill the socket buffers the rest of a large batch no longer fits in them.
func pipeline(t *testing.T, c *client.Conn, cmds []string, multi []bool) []pipeReply {
	t.Helper()
	werr := make(chan error, 1)
	go func() { werr <- c.Send(strings.Join(cmds, "\r\n") + "\r\n") }()
	replies := make([]pipeReply, 0, len(multi))
	for i := range multi {
		code, line, err := c.ReadStatus()
		if err != nil {
			t.Fatalf("reply %d of %d (%.40q): %v (line %q)", i+1, len(multi), cmds[i], err, line)
		}
		r := pipeReply{code: code, status: line}
		if multi[i] {
			if r.lines, err = c.TryReadDotLines(); err != nil {
				t.Fatalf("reply %d (%.40q), multi-line: %v", i+1, cmds[i], err)
			}
		}
		replies = append(replies, r)
	}
	select {
	case err := <-werr:
		if err != nil {
			t.Fatalf("write: %v", err)
		}
	case <-time.After(testkit.Deadline()):
		t.Fatalf("write of %d commands not done after the last reply", len(cmds))
	}
	if err := c.ExpectNoPending(1500 * time.Millisecond); err != nil {
		t.Errorf("after the reply to %q: %v", cmds[len(multi)-1], err)
	}
	return replies
}

// pipeReader returns a reader site with one article in e2e.test, and the article's message-id.
func pipeReader(t *testing.T) (*Site, string) {
	t.Helper()
	st, spool := spoolStore(t)
	s := NewReaderSite(t, ReaderOptions{Spool: spool})
	id := msgID(t)
	feedReader(t, s, st, "e2e.test", newArticle(id, "e2e.test", "pipelining", "hello pipeline", ".dot line"))
	c := s.Reader(t)
	testkit.WaitFor(t, testkit.Deadline(), "the article in GROUP e2e.test", func() bool {
		_, line := c.Cmd("GROUP e2e.test")
		f := strings.Fields(line)
		return len(f) > 1 && f[0] == "211" && f[1] != "0"
	})
	c.Cmd("QUIT")
	return s, id
}

// TestPipeliningReaderMixed: a batch of different commands in one write is answered completely and in
// order, errors included, and nothing sent after QUIT is answered.
func TestPipeliningReaderMixed(t *testing.T) {
	s, id := pipeReader(t)
	c := s.Reader(t)
	steps := []struct {
		cmd   string
		code  int
		multi bool
		lines func([]string) bool // the lines of a multi-line response; nil = not checked
	}{
		{"GROUP e2e.test", 211, false, nil},
		{"STAT 1", 223, false, nil},
		{"HEAD 1", 221, true, func(l []string) bool {
			return containsLine(l, "Message-ID: "+id) && !containsLine(l, "hello pipeline")
		}},
		// ".dot line" un-stuffed: on the wire it was "..dot line" (the strict client fails a single dot)
		{"BODY 1", 222, true, func(l []string) bool { return slices.Equal(l, []string{"hello pipeline", ".dot line"}) }},
		{"ARTICLE 1", 220, true, func(l []string) bool {
			return containsLine(l, "Message-ID: "+id) && containsLine(l, "hello pipeline") && containsLine(l, ".dot line")
		}},
		{"OVER 1-1", 224, true, func(l []string) bool { return len(l) == 1 && strings.Contains(l[0], id) }},
		{"LISTGROUP e2e.test", 211, true, func(l []string) bool { return slices.Equal(l, []string{"1"}) }},
		{"DATE", 111, false, nil},
		{"CAPABILITIES", 101, true, func(l []string) bool { return containsLine(l, "VERSION 2") }},
		{"FROBNICATE", 500, false, nil},
		{"STAT 99", 423, false, nil},
		{"STAT " + id, 223, false, nil},
		{"HELP", 100, true, nil},
		{"QUIT", 205, false, nil},
	}
	var cmds []string
	var multi []bool
	for _, st := range steps {
		cmds = append(cmds, st.cmd)
		multi = append(multi, st.multi)
	}
	// data after QUIT in the same write: not answered (pipeline checks that nothing follows the 205)
	cmds = append(cmds, "DATE", "DATE")
	replies := pipeline(t, c, cmds, multi)
	for i, st := range steps {
		r := replies[i]
		if r.code != st.code {
			t.Errorf("%s: %q, want a %d response", st.cmd, r.status, st.code)
		} else if st.lines != nil && !st.lines(r.lines) {
			t.Errorf("%s in the pipeline: %q", st.cmd, r.lines)
		}
	}
}

// TestPipeliningReaderAlternating: successes and failures alternate in one write and every response
// belongs to its command (the client matches them by position).
func TestPipeliningReaderAlternating(t *testing.T) {
	s, _ := pipeReader(t)
	c := s.Reader(t)
	cmds := []string{"GROUP e2e.test"}
	want := []int{211}
	for i := 0; i < 200; i++ {
		cmds = append(cmds, "STAT 1", "STAT 99", "NOSUCHCOMMAND")
		want = append(want, 223, 423, 500)
	}
	cmds = append(cmds, "QUIT")
	want = append(want, 205)
	replies := pipeline(t, c, cmds, make([]bool, len(cmds)))
	for i := range want {
		if replies[i].code != want[i] {
			t.Fatalf("response %d to %q: %q, want %d", i, cmds[i], replies[i].status, want[i])
		}
	}
}

// TestPipeliningReaderLarge: large batches in one write. dreaderd reads its input when it needs the next
// command, a few KB at a time (4 KB mbufs, 8 KB with BIG_MBUF), so most of a batch waits in the socket
// buffers and many commands straddle the end of one mbuf and the start of the next: 500 whole articles,
// and 30,000 DATE commands (about 180 KB). Every command is answered, in order.
func TestPipeliningReaderLarge(t *testing.T) {
	s, id := pipeReader(t)
	t.Run("articles", func(t *testing.T) {
		c := s.Reader(t)
		cmds := []string{"GROUP e2e.test"}
		multi := []bool{false}
		for i := 0; i < 500; i++ {
			cmds = append(cmds, "ARTICLE 1")
			multi = append(multi, true)
		}
		cmds = append(cmds, "QUIT")
		multi = append(multi, false)
		replies := pipeline(t, c, cmds, multi)
		for i, r := range replies[1:501] {
			if !strings.HasPrefix(r.status, "220 1 "+id) {
				t.Fatalf("article %d: %q", i, r.status)
			}
			if !containsLine(r.lines, "hello pipeline") || !containsLine(r.lines, ".dot line") {
				t.Fatalf("article %d: %q", i, r.lines)
			}
		}
		if replies[501].code != 205 {
			t.Errorf("QUIT: %q", replies[501].status)
		}
	})
	t.Run("commands", func(t *testing.T) {
		c := s.Reader(t)
		var cmds []string
		for i := 0; i < 30000; i++ {
			cmds = append(cmds, "DATE")
		}
		cmds = append(cmds, "QUIT")
		replies := pipeline(t, c, cmds, make([]bool, len(cmds)))
		for i, r := range replies[:30000] {
			if r.code != 111 {
				t.Fatalf("response %d: %q", i, r.status)
			}
		}
		if replies[30000].code != 205 {
			t.Errorf("QUIT: %q", replies[30000].status)
		}
	})
}

// TestPipeliningFeederStreaming (RFC 4644): MODE STREAM, hundreds of CHECK and interleaved TAKETHIS
// with their articles in one write; every response comes in order with its message-id, the articles
// taken are in the history afterwards, and the ones only checked are not.
func TestPipeliningFeederStreaming(t *testing.T) {
	s := NewSite(t, SiteOptions{})
	c := s.dialFrom(t, PeerIP)
	cmds := []string{"MODE STREAM"}
	want := []string{"203"}
	for i := 0; i < 300; i++ {
		cmds = append(cmds, fmt.Sprintf("CHECK <chk%d@e2e.test>", i))
		want = append(want, fmt.Sprintf("238 <chk%d@e2e.test>", i))
	}
	for i := 0; i < 50; i++ {
		id := fmt.Sprintf("<take%d@e2e.test>", i)
		// the article follows its TAKETHIS in the same write; the dot ends it
		art := strings.TrimSuffix(client.Stuff(newArticle(id, "e2e.test", "take", "body")), "\r\n")
		cmds = append(cmds, "TAKETHIS "+id+"\r\n"+art, fmt.Sprintf("CHECK <after%d@e2e.test>", i))
		want = append(want, "239 "+id, fmt.Sprintf("238 <after%d@e2e.test>", i))
	}
	cmds = append(cmds, "QUIT")
	want = append(want, "205")
	replies := pipeline(t, c, cmds, make([]bool, len(cmds)))
	for i, r := range replies {
		if r.status != want[i] && !strings.HasPrefix(r.status, want[i]+" ") {
			t.Fatalf("response %d: %q, want %q...", i, r.status, want[i])
		}
	}

	// every article taken is in the history now, and none of those only checked
	var ids []string
	for i := 0; i < 50; i++ {
		ids = append(ids, fmt.Sprintf("<take%d@e2e.test>", i))
	}
	taken := len(ids)
	for i := 0; i < 300; i++ {
		ids = append(ids, fmt.Sprintf("<chk%d@e2e.test>", i))
	}
	for i := 0; i < 50; i++ {
		ids = append(ids, fmt.Sprintf("<after%d@e2e.test>", i))
	}
	cmds = []string{"MODE STREAM"}
	for _, id := range ids {
		cmds = append(cmds, "CHECK "+id)
	}
	cmds = append(cmds, "QUIT")
	replies = pipeline(t, s.dialFrom(t, PeerIP), cmds, make([]bool, len(cmds)))
	for i, id := range ids {
		r := replies[1+i]
		if f := strings.Fields(r.status); len(f) < 2 || f[1] != id {
			t.Errorf("CHECK %s: %q, about another message-id", id, r.status)
		} else if i < taken && r.code != 438 {
			t.Errorf("CHECK %s, taken before: %q, want 438", id, r.status)
		} else if i >= taken && r.code == 438 {
			t.Errorf("CHECK %s, only checked before: %q, want it not in the history", id, r.status)
		}
	}
}

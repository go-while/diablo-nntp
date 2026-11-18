package e2e

// dreaderd: the feed (NNFeedOverview) frees the Supersedes value of every article and the
// earlier copy of a header that an article carries more than once (D-232). Before, these
// stayed allocated in the feed connection's memory pool until the connection closed: every
// superseding article added a little, and an article with many Supersedes:, Subject:,
// Newsgroups:, Date:, Message-ID:, Control: or Xref: (xref slave mode only) lines added
// about 32 to 40 bytes per line in our runs, without limit.

import (
	"bufio"
	"os"
	"runtime"
	"strconv"
	"strings"
	"testing"

	testkit "github.com/go-while/nntp-testkit"
	"github.com/go-while/nntp-testkit/client"
)

// feedTreeAnonKB adds up RssAnon (kB: private anonymous memory, where the connection pools
// live; not the mapped overview files) of pid and every process below it (Linux /proc).
func feedTreeAnonKB(t *testing.T, pid int) int64 {
	t.Helper()
	ents, err := os.ReadDir("/proc")
	if err != nil {
		t.Fatalf("cannot read /proc: %v", err)
	}
	parent := map[int]int{}
	for _, e := range ents {
		p, err := strconv.Atoi(e.Name())
		if err != nil {
			continue
		}
		b, err := os.ReadFile("/proc/" + e.Name() + "/stat")
		if err != nil {
			continue
		}
		s := string(b)
		i := strings.LastIndexByte(s, ')')
		if i < 0 {
			continue
		}
		if f := strings.Fields(s[i+1:]); len(f) > 1 {
			parent[p], _ = strconv.Atoi(f[1])
		}
	}
	in := map[int]bool{pid: true}
	for changed := true; changed; {
		changed = false
		for p, pp := range parent {
			if !in[p] && in[pp] {
				in[p] = true
				changed = true
			}
		}
	}
	var total int64
	seen := false
	for p := range in {
		f, err := os.Open("/proc/" + strconv.Itoa(p) + "/status")
		if err != nil {
			continue
		}
		sc := bufio.NewScanner(f)
		for sc.Scan() {
			if v, ok := strings.CutPrefix(sc.Text(), "RssAnon:"); ok {
				kb, err := strconv.ParseInt(strings.TrimSuffix(strings.TrimSpace(v), " kB"), 10, 64)
				if err == nil {
					total += kb
					seen = true
				}
			}
		}
		f.Close()
	}
	if !seen {
		t.Fatalf("no RssAnon in /proc/PID/status of dreaderd (pid %d) or its forks", pid)
	}
	return total
}

// feedMemSite is a reader site with one feed connection that stays open, and the number of
// articles fed to e2e.test so far.
type feedMemSite struct {
	s    *Site
	p    *client.Conn
	n    int    // articles that take a number in e2e.test
	last string // the last of them
}

func newFeedMemSite(t *testing.T, conf map[string]string) *feedMemSite {
	_, spool := spoolStore(t)
	s := NewReaderSite(t, ReaderOptions{Spool: spool, Config: conf})
	return &feedMemSite{s: s, p: s.dialFrom(t, PeerIP)}
}

// feed sends count articles over the feed connection; extra adds header lines to article i.
// numbered: the article takes a number in e2e.test (not a control message).
func (m *feedMemSite) feed(t *testing.T, count int, numbered bool, extra func(i int, id string, head []string) []string) {
	t.Helper()
	for range count {
		id := msgID(t)
		i := m.n + 1
		head, body := splitArticle(newArticle(id, "e2e.test", "feed memory "+strconv.Itoa(i), "body"))
		head = extra(i, id, head)
		art := strings.Join(append(append(head, ""), body...), "\r\n") + "\r\n"
		m.p.ExpectIhave(235, id, []byte(art))
		if numbered {
			m.n = i
			m.last = id
		}
	}
}

// part feeds count articles and checks that the private memory of dreaderd and its forks
// grew by at most limitKB.
func (m *feedMemSite) part(t *testing.T, what string, count, limitKB int, numbered bool, extra func(i int, id string, head []string) []string) {
	t.Helper()
	r0 := feedTreeAnonKB(t, m.s.Proc.Pid())
	m.feed(t, count, numbered, extra)
	r1 := feedTreeAnonKB(t, m.s.Proc.Pid())
	t.Logf("RssAnon of dreaderd and its forks: %d kB before, %d kB after %d articles with %s (%+d kB)", r0, r1, count, what, r1-r0)
	if d := r1 - r0; d > int64(limitKB) {
		t.Errorf("the feed's memory grew by %d kB over %d articles with %s (limit %d kB, about %d bytes per article)", d, count, what, limitKB, d*1024/int64(count))
	}
}

// check waits until e2e.test has every numbered article and OVER of the last one has its
// Message-ID, then closes the connections.
func (m *feedMemSite) check(t *testing.T) {
	t.Helper()
	c := m.s.Reader(t)
	testkit.WaitFor(t, testkit.Deadline(), "the high mark of e2e.test at "+strconv.Itoa(m.n), func() bool {
		_, line := c.Cmd("GROUP e2e.test")
		f := strings.Fields(line)
		return len(f) > 3 && f[0] == "211" && f[3] == strconv.Itoa(m.n)
	})
	c.ExpectCmd(224, "OVER %d", m.n)
	if got := c.ReadDotLines(); len(got) != 1 || !strings.Contains(got[0], m.last) {
		t.Errorf("OVER %d: %.200q, want the last article %s", m.n, got, m.last)
	}
	c.ExpectCmd(205, "QUIT")
	m.p.ExpectCmd(205, "QUIT")
}

// TestReaderFeedRepeatedHeadersMemory: over one feed connection, articles to settle, then
// 800 articles with 500 Supersedes: lines each, 800 with 500 repeated lines each (Subject:,
// Newsgroups:, Date: and Message-ID: in turn), and 600 control messages with 500 Control: lines each
// (refused: a control message with Control: twice is not carried out). A second site in
// xref slave mode (where dreaderd keeps the Xref: lines of a feed) gets 600 articles with 500
// Xref: lines each. The private memory of dreaderd and its forks must not grow with the
// number of articles in any part. Without the fix each part added about 32 to 40 bytes per
// line in our runs (800 x 500 x 32 bytes, about 12 MB, and about 15 MB in the mixed part;
// 600 x 500 x 32, about 9 MB); the limit is 4 MiB per part.
func TestReaderFeedRepeatedHeadersMemory(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("reads /proc")
	}
	const (
		settle  = 200
		lines   = 500
		limitKB = 4 * 1024
	)
	supersedes := func(i int, id string, head []string) []string {
		for k := range lines {
			head = append(head, "Supersedes: <old."+strconv.Itoa(i)+"."+strconv.Itoa(k)+"@e2e.test>")
		}
		return head
	}
	repeated := func(i int, id string, head []string) []string {
		date := headerValue(head, "Date")
		for k := range lines {
			switch k % 4 {
			case 0:
				head = append(head, "Subject: feed memory "+strconv.Itoa(i)+" copy "+strconv.Itoa(k))
			case 1:
				head = append(head, "Newsgroups: e2e.test")
			case 2:
				head = append(head, "Date: "+date)
			default:
				head = append(head, "Message-ID: "+id)
			}
		}
		return head
	}
	control := func(i int, id string, head []string) []string {
		for k := range lines {
			head = append(head, "Control: cancel <old."+strconv.Itoa(i)+"."+strconv.Itoa(k)+"@e2e.test>")
		}
		return head
	}

	m := newFeedMemSite(t, nil)
	m.feed(t, settle, true, supersedes)
	m.feed(t, settle/2, true, repeated)
	m.feed(t, settle/2, false, control)
	m.part(t, strconv.Itoa(lines)+" Supersedes: lines", 800, limitKB, true, supersedes)
	m.part(t, strconv.Itoa(lines)+" repeated Subject:, Newsgroups:, Date: and Message-ID: lines", 800, limitKB, true, repeated)
	m.part(t, strconv.Itoa(lines)+" Control: lines", 600, limitKB, false, control)
	m.check(t)

	x := newFeedMemSite(t, map[string]string{"readerxrefslavehost": PathHost})
	xref := func(i int, id string, head []string) []string {
		for range lines {
			head = append(head, "Xref: "+PathHost+" e2e.test:"+strconv.Itoa(i))
		}
		return head
	}
	x.feed(t, settle, true, xref)
	x.part(t, strconv.Itoa(lines)+" Xref: lines", 600, limitKB, true, xref)
	x.check(t)
}

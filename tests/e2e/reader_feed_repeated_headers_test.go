package e2e

// dreaderd: the feed (NNFeedOverview) frees the Supersedes value of every article and the
// earlier copy of a header that an article carries more than once (D-232). Before, these
// stayed allocated in the feed connection's memory pool until the connection closed: every
// superseding article added a little, and an article with many Supersedes:, Subject:,
// Newsgroups: or Date: lines added about 32 bytes per line, without limit.

import (
	"bufio"
	"os"
	"runtime"
	"strconv"
	"strings"
	"testing"

	testkit "github.com/go-while/nntp-testkit"
)

// feedTreeAnonKB adds up RssAnon (kB: private anonymous memory, where the connection pools
// live; not the mapped overview files) of pid and every process below it (Linux /proc).
func feedTreeAnonKB(t *testing.T, pid int) int64 {
	t.Helper()
	ents, err := os.ReadDir("/proc")
	if err != nil {
		t.Skipf("no /proc: %v", err)
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
		t.Skip("no RssAnon in /proc/PID/status (Linux before 4.5)")
	}
	return total
}

// TestReaderFeedRepeatedHeadersMemory: over one feed connection, 200 articles to settle,
// then 800 articles with 500 Supersedes: lines each, then 800 articles with 500 repeated
// Subject:, Newsgroups: and Date: lines each. The private memory of dreaderd and its forks
// must not grow with the number of articles in either phase. Without the fix each phase
// adds about 12-13 MB (800 x 500 x 32 bytes); the limit is 4 MiB per phase.
func TestReaderFeedRepeatedHeadersMemory(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("reads /proc")
	}
	const (
		settle  = 200
		perPart = 800
		lines   = 500
		limitKB = 4 * 1024
	)
	_, spool := spoolStore(t)
	s := NewReaderSite(t, ReaderOptions{Spool: spool})
	pid := s.Proc.Pid()
	p := s.dialFrom(t, PeerIP)

	n := 0
	var last string
	feed := func(count int, extra func(i int, head []string) []string) {
		t.Helper()
		for range count {
			n++
			id := msgID(t)
			last = id
			head, body := splitArticle(newArticle(id, "e2e.test", "feed memory "+strconv.Itoa(n), "body"))
			head = extra(n, head)
			art := strings.Join(append(append(head, ""), body...), "\r\n") + "\r\n"
			p.ExpectIhave(235, id, []byte(art))
		}
	}
	supersedes := func(i int, head []string) []string {
		for k := range lines {
			head = append(head, "Supersedes: <old."+strconv.Itoa(i)+"."+strconv.Itoa(k)+"@e2e.test>")
		}
		return head
	}
	repeated := func(i int, head []string) []string {
		date := headerValue(head, "Date")
		for k := range lines {
			switch k % 3 {
			case 0:
				head = append(head, "Subject: feed memory "+strconv.Itoa(i)+" copy "+strconv.Itoa(k))
			case 1:
				head = append(head, "Newsgroups: e2e.test")
			default:
				head = append(head, "Date: "+date)
			}
		}
		return head
	}

	feed(settle, supersedes)
	feed(settle/2, repeated)
	r0 := feedTreeAnonKB(t, pid)
	feed(perPart, supersedes)
	r1 := feedTreeAnonKB(t, pid)
	feed(perPart, repeated)
	r2 := feedTreeAnonKB(t, pid)
	t.Logf("RssAnon of dreaderd and its forks (kB): %d before, %d after %d articles with %d Supersedes: (%+d), %d after %d with %d repeated headers (%+d)",
		r0, r1, perPart, lines, r1-r0, r2, perPart, lines, r2-r1)
	if d := r1 - r0; d > limitKB {
		t.Errorf("the feed's memory grew by %d kB over %d articles with %d Supersedes: lines each (limit %d kB, about %d bytes per article)", d, perPart, lines, limitKB, d*1024/perPart)
	}
	if d := r2 - r1; d > limitKB {
		t.Errorf("the feed's memory grew by %d kB over %d articles with %d repeated Subject:, Newsgroups: and Date: lines each (limit %d kB, about %d bytes per article)", d, perPart, lines, limitKB, d*1024/perPart)
	}

	// the articles were taken and numbered one after the other
	c := s.Reader(t)
	testkit.WaitFor(t, testkit.Deadline(), "the high mark of e2e.test at "+strconv.Itoa(n), func() bool {
		_, line := c.Cmd("GROUP e2e.test")
		f := strings.Fields(line)
		return len(f) > 3 && f[0] == "211" && f[3] == strconv.Itoa(n)
	})
	c.ExpectCmd(224, "OVER %d", n)
	if got := c.ReadDotLines(); len(got) != 1 || !strings.Contains(got[0], last) {
		t.Errorf("OVER %d: %.200q, want the last article %s", n, got, last)
	}
	c.ExpectCmd(205, "QUIT")
	p.ExpectCmd(205, "QUIT")
}

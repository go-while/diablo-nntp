package e2e

// Deep review round 2, M4 (reproduction): NNFeedOverview (dreaderd/group.c) never frees Supersedes
// and does not free the earlier copy of a header that is repeated; the feed connection's memory
// pool grows until the connection closes.

import (
	"bufio"
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

// r2TreeRSS adds up VmRSS (kB) of pid and every process below it (Linux, /proc).
func r2TreeRSS(t *testing.T, pid int) int64 {
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
		f := strings.Fields(s[i+1:])
		if len(f) > 1 {
			pp, _ := strconv.Atoi(f[1])
			parent[p] = pp
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
	for p := range in {
		f, err := os.Open("/proc/" + strconv.Itoa(p) + "/status")
		if err != nil {
			continue
		}
		sc := bufio.NewScanner(f)
		for sc.Scan() {
			if v, ok := strings.CutPrefix(sc.Text(), "VmRSS:"); ok {
				var kb int64
				fmt.Sscanf(strings.TrimSpace(v), "%d", &kb)
				total += kb
			}
		}
		f.Close()
	}
	return total
}

// TestR2ReproSupersedesLeak: 2000 articles with 500 Supersedes lines each over one feed
// connection. The memory of the reader's processes must not grow with the number of articles.
func TestR2ReproSupersedesLeak(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("reads /proc")
	}
	_, spool := spoolStore(t)
	s := NewReaderSite(t, ReaderOptions{Spool: spool})
	pid := s.Proc.Pid()
	p := s.dialFrom(t, PeerIP)
	feed := func(from, to int) {
		for i := from; i < to; i++ {
			id := msgID(t)
			a := newArticle(id, "e2e.test", "supersedes leak "+strconv.Itoa(i), "body")
			head, body := splitArticle(a)
			for k := range 500 {
				head = append(head, "Supersedes: <old."+strconv.Itoa(i)+"."+strconv.Itoa(k)+"@e2e.test>")
			}
			art := strings.Join(append(append(head, ""), body...), "\r\n") + "\r\n"
			p.ExpectIhave(235, id, []byte(art))
		}
	}
	feed(0, 200)
	r0 := r2TreeRSS(t, pid)
	feed(200, 1100)
	r1 := r2TreeRSS(t, pid)
	feed(1100, 2000)
	r2 := r2TreeRSS(t, pid)
	t.Logf("VmRSS of dreaderd and its children (kB): after 200 articles %d, after 1100 %d, after 2000 %d (growth %d kB)", r0, r1, r2, r2-r0)
	if r2-r0 > 12*1024 {
		t.Errorf("the reader's memory grew by %d kB over 1800 articles with 500 Supersedes lines each (limit 12288 kB)", r2-r0)
	}
}

package e2e

import (
	"bytes"
	"compress/zlib"
	"os/exec"
	"strconv"
	"strings"
	"testing"
)

// descendantRSS returns the largest resident size in KB among all descendants of pid.
func descendantRSS(pid int) (max int) {
	out, _ := exec.Command("ps", "-A", "-o", "pid,ppid,rss").Output()
	kids := map[int][]int{}
	rss := map[int]int{}
	for _, l := range strings.Split(string(out), "\n") {
		f := strings.Fields(l)
		if len(f) != 3 || f[0] == "PID" {
			continue
		}
		p, _ := strconv.Atoi(f[0])
		pp, _ := strconv.Atoi(f[1])
		rss[p], _ = strconv.Atoi(f[2])
		kids[pp] = append(kids[pp], p)
	}
	todo := []int{pid}
	for len(todo) > 0 {
		p := todo[0]
		todo = append(todo[1:], kids[p]...)
		if p != pid && rss[p] > max {
			max = rss[p]
		}
	}
	return max
}

// TestFeederModeCompressRepeated: a peer that repeats MODE COMPRESS (each time as a fresh
// deflate stream) must not make the session grow without bound. Before the fix every
// command allocated a new decompression state, about 22 KB each, and never freed it.
func TestFeederModeCompressRepeated(t *testing.T) {
	s := NewSite(t, SiteOptions{})
	p := s.Peer(t)
	expect(t, p, 207, "MODE COMPRESS")
	before := descendantRSS(s.Proc.Pid())
	const rounds = 600
	sent := 0
	for i := 0; i < rounds; i++ {
		var bb bytes.Buffer
		zw := zlib.NewWriter(&bb)
		zw.Write([]byte("MODE COMPRESS\r\n"))
		zw.Flush()
		if _, err := p.Conn.Write(bb.Bytes()); err != nil {
			break
		}
		line, err := p.ReadLine()
		if err != nil {
			break // the server may drop a peer that restarts the stream
		}
		if !strings.HasPrefix(line, "207") && !strings.HasPrefix(line, "502") {
			t.Fatalf("round %d: unexpected reply %q", i, line)
		}
		sent++
	}
	after := descendantRSS(s.Proc.Pid())
	t.Logf("%d commands answered, session RSS %d KB -> %d KB", sent, before, after)
	if after-before > 3*1024 {
		t.Errorf("session grew by %d KB after repeated MODE COMPRESS", after-before)
	}
}

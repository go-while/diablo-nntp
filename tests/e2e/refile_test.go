package e2e

import (
	"net"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"
)

// TestDnewslinkRefileKeepsFlags (D-63): when the peer goes away with streamed entries still
// pending, dnewslink writes them back to the queue file. Each entry must come back once, with
// its ' H' (header only) and ' C<size>' (compressed spool) flags. Fails on main: the pending
// entries lost both flags, and the one being sent was written a second time with them.
func TestDnewslinkRefileKeepsFlags(t *testing.T) {
	s := NewSite(t, SiteOptions{NoStart: true})
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	port := ln.Addr().(*net.TCPAddr).Port
	go func() {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		defer c.Close()
		c.Write([]byte("200 fake\r\n"))
		buf := make([]byte, 4096)
		var got string
		for {
			c.SetReadDeadline(time.Now().Add(10 * time.Second))
			n, err := c.Read(buf)
			if err != nil {
				return
			}
			got += string(buf[:n])
			for {
				i := strings.Index(got, "\n")
				if i < 0 {
					break
				}
				line := strings.ToLower(strings.TrimSpace(got[:i]))
				got = got[i+1:]
				switch {
				case strings.HasPrefix(line, "mode headfeed"):
					c.Write([]byte("250 ok\r\n"))
				case strings.HasPrefix(line, "mode stream"):
					c.Write([]byte("203 stream ok\r\n"))
				case strings.HasPrefix(line, "check"):
					return // go away with the CHECKs unanswered
				}
			}
		}
	}()
	q := "D.00000001/B.0001 <a1@e2e.test> 0,100 H C50\nD.00000001/B.0001 <a2@e2e.test> 100,100 H C50\n"
	if err := os.WriteFile(s.Dir+"/dqueue/out.S00000", []byte(q), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := s.TryRun("dnewslink", "-H", "-N", "1", "-S", "0", "-b", "out.S%05d", "-h", "127.0.0.1",
		"-P", strconv.Itoa(port), "-t", "5")
	t.Logf("dnewslink: err=%v\n%s", err, out)

	b, rerr := os.ReadFile(s.Dir + "/dqueue/out.S00000")
	if rerr != nil {
		t.Fatal(rerr)
	}
	var lines []string
	for _, l := range strings.Split(string(b), "\n") {
		if strings.TrimSpace(l) != "" {
			lines = append(lines, l)
		}
	}
	if len(lines) != 2 {
		t.Fatalf("queue file has %d entries, want the 2 that were queued:\n%s", len(lines), b)
	}
	for i, id := range []string{"<a1@e2e.test>", "<a2@e2e.test>"} {
		f := strings.Fields(lines[i])
		if len(f) < 3 || f[1] != id || !strings.Contains(lines[i], " H") || !strings.Contains(lines[i], "C50") {
			t.Errorf("entry %d is %q, want %s with its H and C50 flags", i+1, lines[i], id)
		}
	}
}

package e2e

import (
	"bufio"
	"net"
	"strings"
	"testing"
	"time"
)

// TestDnewsfeedsMaxconnect (D-59): `maxconnect N` in a dnewsfeeds label lets N parallel
// connections of a host in, not N+1. Fails on main: with maxconnect 2 the third connection
// was accepted too, and only the fourth refused.
func TestDnewsfeedsMaxconnect(t *testing.T) {
	s := NewSite(t, SiteOptions{Newsfeeds: GlobalLabel + `label peer
    inhost ` + PeerIP + `
    alias e2e.peer
    addgroup *
    maxconnect 2
end
`})
	var conns []net.Conn
	defer func() {
		for _, c := range conns {
			c.Close()
		}
	}()
	for i := 1; i <= 3; i++ {
		d := net.Dialer{LocalAddr: &net.TCPAddr{IP: net.ParseIP(PeerIP)}, Timeout: 5 * time.Second}
		c, err := d.Dial("tcp", s.Addr)
		if err != nil {
			t.Fatalf("connection %d: %v", i, err)
		}
		conns = append(conns, c)
		c.SetReadDeadline(time.Now().Add(5 * time.Second))
		line, err := bufio.NewReader(c).ReadString('\n')
		if err != nil {
			t.Fatalf("connection %d: greeting: %v", i, err)
		}
		want := "20"
		if i == 3 {
			want = "502"
		}
		if !strings.HasPrefix(line, want) {
			t.Errorf("connection %d: %q, want %s", i, strings.TrimSpace(line), want)
		}
		if i == 3 && !strings.Contains(line, "limit is 2") {
			t.Errorf("connection 3: %q does not name the limit", strings.TrimSpace(line))
		}
	}
}

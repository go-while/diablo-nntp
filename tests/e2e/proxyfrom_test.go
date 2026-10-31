package e2e

import (
	"strings"
	"testing"
)

// TestReaderProxyFromAllowlist (GHSA-cp62-cqpx-g9mx): a readerdef with useproxied takes the
// client address from the first line the client sends. With proxyfrom the line is accepted only
// from the listed proxy addresses; any other peer gets 400 and is closed.
func TestReaderProxyFromAllowlist(t *testing.T) {
	for _, tc := range []struct {
		name, from string
		refused    bool
	}{
		{"other proxy", "192.0.2.1", true},
		{"network without the peer", "10.0.0.0/8", true},
		{"peer itself", "127.0.0.1", false},
		{"network with the peer", "127.0.0.0/8", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, spool := spoolStore(t)
			access := strings.Replace(DefaultReaderAccess, "readerdef rp\n",
				"readerdef rp\n    useproxied yes\n    proxyfrom "+tc.from+"\n", 1)
			s := NewReaderSite(t, ReaderOptions{Spool: spool, Access: access})
			c := s.Reader(t)
			code, line := c.Cmd("proxied 10.9.8.7:1234")
			t.Logf("reply %d %s", code, line)
			isRefusal := code == 400 && strings.Contains(line, "Proxy authentication failure")
			if isRefusal != tc.refused {
				t.Errorf("proxied line from the peer with proxyfrom %s: reply %d %q, refused=%v want %v",
					tc.from, code, line, isRefusal, tc.refused)
			}
		})
	}
}

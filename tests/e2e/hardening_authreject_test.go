package e2e

import (
	"strings"
	"testing"
	"time"
)

// TestReaderAuthRejectKeepsMaster: the login of a user whose entry in a file auth backend
// carries a numeric reject string gets the intended 500 reply and the dreaderd master keeps
// running and serves the next client. Repeated with a fresh dreaderd each time.
func TestReaderAuthRejectKeepsMaster(t *testing.T) {
	for i := 0; i < 8; i++ {
		_, spool := spoolStore(t)
		s := NewReaderSite(t, ReaderOptions{Spool: spool, NoStart: true})
		s.WriteFile(t, "userpass", "bad:pw:600 1 500 Your connection is rejected\ngood:pw2\n")
		s.WriteFile(t, "dreader.access", `vserverdef DEFAULT
    hostname `+PathHost+`
end
authdef f
    file `+s.Dir+`/userpass
end
readerdef rf
    read yes
    auth f
end
access 127.0.0.1 rf
`)
		startReader(t, s, "[127.0.0.1]:"+strings.Split(s.Addr, ":")[1])

		c := s.Reader(t)
		expect(t, c, 381, "AUTHINFO USER bad")
		send(t, c, "AUTHINFO PASS pw\r\n")
		code, line, err := c.ReadStatus()
		if err == nil && code != 500 {
			t.Errorf("attempt %d: %d %q, want 500", i, code, line)
		}
		c.Close()
		time.Sleep(500 * time.Millisecond)
		select {
		case <-s.Proc.Done():
			t.Fatalf("attempt %d: the dreaderd master exited: %v", i, s.Proc.Wait())
		default:
		}
		c = s.Reader(t)
		expect(t, c, 381, "AUTHINFO USER good")
		expect(t, c, 281, "AUTHINFO PASS pw2")
		c.Close()
	}
}

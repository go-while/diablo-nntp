package e2e

import (
	"io"
	"net"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	testkit "github.com/go-while/nntp-testkit"
	"github.com/go-while/nntp-testkit/client"
)

// netRemoteServer is a NetRemote server that takes every request: it sends its time (12
// digits and a newline), reads the 13-byte size line and the encrypted request, answers
// "1\n" (a login is accepted; a session-end report reads no answer) and closes. It counts the
// requests it got whole.
func netRemoteServer(t *testing.T) (port int, count func() int) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	var mu sync.Mutex
	n := 0
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				c.SetDeadline(time.Now().Add(10 * time.Second))
				if _, err := io.WriteString(c, "000000000000\n"); err != nil {
					return
				}
				hdr := make([]byte, 13)
				if _, err := io.ReadFull(c, hdr); err != nil {
					return
				}
				size, err := strconv.Atoi(strings.TrimSpace(string(hdr)))
				if err != nil || size <= 0 || size > 65536 {
					return
				}
				if _, err := io.ReadFull(c, make([]byte, size)); err != nil {
					return
				}
				mu.Lock()
				n++
				mu.Unlock()
				io.WriteString(c, "1\n")
			}(c)
		}
	}()
	return ln.Addr().(*net.TCPAddr).Port, func() int {
		mu.Lock()
		defer mu.Unlock()
		return n
	}
}

// TestNetRemoteAccountingToAuthdef (D-173): at the end of a session that logged in with a
// password, the DNS worker sends the session's accounting to the netremote server of the
// authdef the user logged in through, and sends nothing for an authdef without one. pat
// logs in on an authdef with user and pass only, then x@slow through the NetRemote server;
// with one DNS worker the requests are handled in order, so the server sees exactly x's login
// and x's session end. Fails before the fix: every session end went to
// "Password@accounting.server:8998", so the server saw only the login, and pat's session was
// sent there too.
func TestNetRemoteAccountingToAuthdef(t *testing.T) {
	builtWith(t, "NetRemote socket connect", "NetRemote support (NETREMOTE_ENABLED in lib/vendor.h)")
	port, count := netRemoteServer(t)
	s := authTimeoutSite(t, slowAuthAccess("netremote secret@127.0.0.1:"+strconv.Itoa(port)))

	c := client.DialWith(t, s.Addr, 2*authTimeoutBound)
	c.ExpectCmd(381, "AUTHINFO USER pat")
	c.ExpectCmd(281, "AUTHINFO PASS patspass")
	c.ExpectCmd(205, "QUIT")
	c.Close()

	c = client.DialWith(t, s.Addr, 2*authTimeoutBound)
	c.ExpectCmd(381, "AUTHINFO USER x@slow")
	c.ExpectCmd(281, "AUTHINFO PASS pw")
	c.ExpectCmd(205, "QUIT")
	c.Close()

	testkit.WaitFor(t, 10*time.Second, "the NetRemote server to get x's login and session end", func() bool {
		return count() >= 2
	})
	time.Sleep(time.Second)
	if n := count(); n != 2 {
		t.Errorf("the NetRemote server got %d requests, want 2 (x's login and x's session end)", n)
	}
	if log := generalLog(s); strings.Contains(log, "accounting.server") {
		t.Errorf("a session end went to the placeholder accounting.server:\n%s", log)
	}
}

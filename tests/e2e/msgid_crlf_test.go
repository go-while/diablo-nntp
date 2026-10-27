package e2e

import (
	"bufio"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/go-while/nntp-testkit/fakeserver"
)

// TestMsgIdControlCharacters (GHSA-4w53-m8r4-5cv2): lib/msgid.c tested *s, always '<', instead of
// s[i] for CR and LF, so an id with a CR inside passed MsgId(). diablo splits its command lines
// at CR and LF before MsgId() sees them, so the bug shows in dreaderd: a feed peer's CHECK got 238,
// and a POST with such a Message-ID header was accepted, stored and sent on. Both are refused now.
func TestMsgIdControlCharacters(t *testing.T) {
	post := fakeserver.New(t, fakeserver.Options{Handler: fakeserver.PeerHandler(&fakeserver.PeerOptions{})})
	_, spool := spoolStore(t)
	s := NewReaderSite(t, ReaderOptions{Spool: spool, Post: post})

	t.Run("CHECK from a feed peer", func(t *testing.T) {
		d := net.Dialer{LocalAddr: &net.TCPAddr{IP: net.ParseIP(PeerIP)}, Timeout: 3 * time.Second}
		nc, err := d.Dial("tcp", s.Addr)
		if err != nil {
			t.Fatal(err)
		}
		defer nc.Close()
		nc.SetDeadline(time.Now().Add(10 * time.Second))
		r := bufio.NewReader(nc)
		line := func() string {
			l, err := r.ReadString('\n')
			if err != nil {
				t.Fatalf("read: %v", err)
			}
			return l
		}
		line() // greeting
		send := func(cmd string) string {
			if _, err := nc.Write([]byte(cmd + "\r\n")); err != nil {
				t.Fatal(err)
			}
			return line()
		}
		if got := send("MODE STREAM"); !strings.HasPrefix(got, "203") {
			t.Fatalf("MODE STREAM: %q", got)
		}
		if got := send("CHECK <plain@e2e.test>"); !strings.HasPrefix(got, "238 <plain@e2e.test>") {
			t.Errorf("CHECK of a plain id: %q, want 238", got)
		}
		if got := send("CHECK <a\rb@e2e.test>"); strings.HasPrefix(got, "238") {
			t.Errorf("CHECK of an id with a CR inside was accepted: %q", got)
		}
		// the connection is still in step
		if got := send("CHECK <plain2@e2e.test>"); !strings.HasPrefix(got, "238 <plain2@e2e.test>") {
			t.Errorf("CHECK after it: %q, want 238", got)
		}
	})

	t.Run("POST", func(t *testing.T) {
		c := s.Reader(t)
		post := func(id string) (int, string) {
			expect(t, c, 340, "POST")
			send(t, c, wire([]string{
				"From: E2E Poster <poster@example.invalid>",
				"Newsgroups: e2e.test",
				"Subject: control characters in an id",
				"Message-ID: " + id,
				"",
				"body",
			}))
			code, line, err := c.ReadStatus()
			if err != nil {
				t.Fatal(err)
			}
			return code, line
		}
		if code, line := post("<plain@e2e.test>"); code != 240 {
			t.Fatalf("POST with a plain id: %d %q, want 240", code, line)
		}
		for _, id := range []string{"<a\rb@e2e.test>", "<a b@e2e.test>"} {
			if code, line := post(id); code != 441 {
				t.Errorf("POST with the id %q: %d %q, want 441", id, code, line)
			}
		}
	})
}

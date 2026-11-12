package e2e

import (
	"strings"
	"testing"
	"time"

	testkit "github.com/go-while/nntp-testkit"
)

// takethisRefused connects from "from", sends setup (each line expected to get an answer
// starting with the given code), then in one write the TAKETHIS line, an article whose body
// has a line "QUIT", its dot, and next; and returns the answer lines that came within 5 s, up
// to the answer of next (one that starts with nextCode) or a 205.
func takethisRefused(t *testing.T, addr, from string, setup [][2]string, line, next, nextCode string) []string {
	t.Helper()
	n := dialNulLine(t, addr, from)
	for _, s := range setup {
		if l, err := n.send(s[0] + "\r\n"); err != nil || !strings.HasPrefix(l, s[1]+" ") {
			t.Fatalf("from %s: %s: %q %v, want %s", from, s[0], l, err, s[1])
		}
	}
	id := "<takethis-refused." + strings.ReplaceAll(t.Name(), "/", ".") + "@e2e.test>"
	raw := line + "\r\n" +
		"Path: e2e.peer!not-for-mail\r\n" +
		"From: <refused@example.invalid>\r\n" +
		"Newsgroups: e2e.test\r\n" +
		"Subject: refused takethis\r\n" +
		"Date: " + time.Now().UTC().Format(time.RFC1123Z) + "\r\n" +
		"Message-ID: " + id + "\r\n" +
		"\r\n" +
		"QUIT\r\n" +
		".\r\n" +
		next + "\r\n"
	n.c.SetWriteDeadline(time.Now().Add(5 * time.Second))
	if _, err := n.c.Write([]byte(raw)); err != nil {
		t.Fatal(err)
	}
	var lines []string
	for {
		l, err := n.line()
		if err != nil {
			break
		}
		lines = append(lines, l)
		if strings.HasPrefix(l, nextCode+" ") || strings.HasPrefix(l, "205 ") {
			break
		}
	}
	return lines
}

// TestTakethisRefusedFraming: a TAKETHIS is followed by its article without waiting for an
// answer (RFC 4644 2.5.2), so a refused TAKETHIS must read and drop the article before it
// answers. dreaderd answered "439 <>" for a missing or invalid message-id, and 480 when the
// feed had not yet authenticated, without reading the article: its lines were taken for
// commands (500 per header line, and a body line "QUIT" ended the session). Now the answer
// comes after the dot (439 with the message-id as sent, its first word, if all of it is
// printable US-ASCII, else "<>"), then the next command's own answer. diablo already read the article of its refused TAKETHIS (control).
//
// The answer is kept with each connection: two feeds of one reader process that drop an
// article at the same time, one refused for a NUL byte (501), one for an invalid message-id
// (439), must each get their own answer.
func TestTakethisRefusedFraming(t *testing.T) {
	stream := [][2]string{{"MODE STREAM", "203"}}
	check := func(t *testing.T, lines []string, first, next string) {
		t.Helper()
		if len(lines) != 2 || lines[0] != first && !strings.HasPrefix(lines[0], first+" ") || !strings.HasPrefix(lines[1], next+" ") {
			t.Errorf("refused TAKETHIS, its article (a body line QUIT), the next command: answers %q, want %q and then %s", lines, first, next)
		}
	}
	t.Run("dreaderd invalid message-id", func(t *testing.T) {
		_, spool := spoolStore(t)
		s := NewReaderSite(t, ReaderOptions{Spool: spool})
		check(t, takethisRefused(t, s.Addr, PeerIP, stream, "TAKETHIS not-a-message-id", "MODE STREAM", "203"),
			"439 not-a-message-id", "203")
	})
	t.Run("dreaderd no message-id", func(t *testing.T) {
		_, spool := spoolStore(t)
		s := NewReaderSite(t, ReaderOptions{Spool: spool})
		check(t, takethisRefused(t, s.Addr, PeerIP, stream, "TAKETHIS", "MODE STREAM", "203"), "439 <>", "203")
	})
	// only printable US-ASCII is echoed: a control byte in an invalid id gives "439 <>"
	t.Run("dreaderd message-id with a control byte", func(t *testing.T) {
		_, spool := spoolStore(t)
		s := NewReaderSite(t, ReaderOptions{Spool: spool})
		check(t, takethisRefused(t, s.Addr, PeerIP, stream, "TAKETHIS <a\x01b\x7fc", "MODE STREAM", "203"), "439 <>", "203")
	})
	t.Run("dreaderd authentication required", func(t *testing.T) {
		access := DefaultReaderAccess + `authdef feedauth
    user feeder
    pass feederpass
end
readerdef feedauth
    feed yes
    auth feedauth
end
access ` + PeerIP + ` feedauth
`
		access = strings.Replace(access, "access "+PeerIP+" feed\n", "", 1)
		_, spool := spoolStore(t)
		s := NewReaderSite(t, ReaderOptions{Spool: spool, Access: access})
		check(t, takethisRefused(t, s.Addr, PeerIP, nil, "TAKETHIS <auth."+t.Name()+"@e2e.test>", "DATE", "111"),
			"480", "111")
	})
	t.Run("dreaderd two discards at once", func(t *testing.T) {
		const other = "127.0.0.5"
		_, spool := spoolStore(t)
		// readerdetaillog on: dreaderd logs "<host> [pid/slot] - dropping the article ..." when
		// a connection enters the discard state; the test waits for both before either dot
		s := NewReaderSite(t, ReaderOptions{Spool: spool, Access: DefaultReaderAccess + "access " + other + " feed\n",
			Config: map[string]string{"readerdetaillog": "on"}})
		dropping := func(host string) bool {
			for _, l := range strings.Split(generalLog(s), "\n") {
				if strings.Contains(l, " "+host+" [") && strings.Contains(l, "] - dropping the article of a refused TAKETHIS") {
					return true
				}
			}
			return false
		}
		article := func(id string) (head, tail string) {
			head = "Path: e2e.peer!not-for-mail\r\nFrom: <two@example.invalid>\r\n"
			tail = "Newsgroups: e2e.test\r\nSubject: two discards\r\n" +
				"Date: " + time.Now().UTC().Format(time.RFC1123Z) + "\r\nMessage-ID: " + id + "\r\n\r\nQUIT\r\n.\r\nDATE\r\n"
			return
		}
		write := func(n *nulLineConn, raw string) {
			t.Helper()
			n.c.SetWriteDeadline(time.Now().Add(5 * time.Second))
			if _, err := n.c.Write([]byte(raw)); err != nil {
				t.Fatal(err)
			}
		}
		answers := func(n *nulLineConn) []string {
			var lines []string
			for len(lines) < 4 {
				l, err := n.line()
				if err != nil {
					break
				}
				lines = append(lines, l)
				if strings.HasPrefix(l, "111 ") || strings.HasPrefix(l, "205 ") {
					break
				}
			}
			return lines
		}
		a := dialNulLine(t, s.Addr, PeerIP)
		b := dialNulLine(t, s.Addr, other)
		for _, n := range []*nulLineConn{a, b} {
			if l, err := n.send("MODE STREAM\r\n"); err != nil || !strings.HasPrefix(l, "203 ") {
				t.Fatalf("MODE STREAM: %q %v", l, err)
			}
		}
		// both in the middle of their article, A refused for a NUL byte, B for its message-id
		aHead, aTail := article("<two.a@e2e.test>")
		bHead, bTail := article("<two.b@e2e.test>")
		write(a, "TAKETHIS <two.a@e2e.test>\x00junk\r\n"+aHead)
		write(b, "TAKETHIS not-an-id\r\n"+bHead)
		// both connections are in the discard state before either dot is sent (the answers
		// come only after the dot, so they cannot be waited for here)
		testkit.WaitFor(t, 10*time.Second, "both connections dropping their article", func() bool {
			return dropping(PeerIP) && dropping(other)
		})
		write(a, aTail)
		write(b, bTail)
		check(t, answers(a), "501", "111")
		check(t, answers(b), "439 not-an-id", "111")
	})
	t.Run("diablo", func(t *testing.T) {
		s := NewSite(t, SiteOptions{})
		check(t, takethisRefused(t, s.Addr, PeerIP, stream, "TAKETHIS not-a-message-id", "MODE STREAM", "203"), "501", "203")
	})
}

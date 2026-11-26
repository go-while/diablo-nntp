package e2e

// D-304 diagnostics: a reader connection that ends without any command line (the client
// reads the greeting and closes) is logged as "connection ended before first command", next to the
// "$ exit" line; a connection that sent a command is not.

import (
	"bufio"
	"net"
	"strings"
	"testing"
	"time"

	testkit "github.com/go-while/nntp-testkit"
)

// TestReaderEOFBeforeFirstCommand: with the command log on (readerdetaillog, the default),
// a client that reads the greeting and closes gets "$ connection ended before first command" in the
// log; a client that sends QUIT gets its "$ exit" line without it. Fails on main: the
// message does not exist.
func TestReaderEOFBeforeFirstCommand(t *testing.T) {
	_, spool := spoolStore(t)
	s := NewReaderSite(t, ReaderOptions{Spool: spool, Config: map[string]string{"readerdetaillog": "on"}})
	count := func(sub string) int { return strings.Count(generalLog(s), sub) }

	// sessions with a command first (the start-up readiness probe is one more of them)
	for i := 0; i < 2; i++ {
		c := s.Reader(t)
		c.ExpectCmd(205, "QUIT")
	}

	// greeting read, then closed without a command
	nc, err := net.DialTimeout("tcp", s.Addr, testkit.Deadline())
	if err != nil {
		t.Fatal(err)
	}
	nc.SetReadDeadline(time.Now().Add(testkit.Deadline()))
	greeting, err := bufio.NewReader(nc).ReadString('\n')
	if err != nil || !strings.HasPrefix(greeting, "200 ") && !strings.HasPrefix(greeting, "201 ") {
		t.Fatalf("greeting: %q %v", greeting, err)
	}
	nc.Close()
	// every session that sent QUIT has its "$ exit" line without the message: in the end
	// there is exactly one message, and one "$ exit" line more than there are QUIT commands
	testkit.WaitFor(t, 10*time.Second, "connection ended before first command", func() bool {
		return count("$ connection ended before first command") >= 1 && count("$ exit articles") >= count("] < QUIT")+1
	})
	time.Sleep(300 * time.Millisecond)
	if n, e, q := count("$ connection ended before first command"), count("$ exit articles"), count("] < QUIT"); n != 1 || e != q+1 {
		t.Fatalf("EOF messages %d (want 1), exit lines %d, QUIT commands %d (want exit = QUIT + 1):\n%s", n, e, q, generalLog(s))
	}
	// a plain close is not a read error
	if n := count("$ read error: "); n != 0 {
		t.Fatalf("%d read error lines after plain closes, want 0:\n%s", n, generalLog(s))
	}
}

// TestReaderReadErrorLogged: a client that reads the greeting and then resets the
// connection (SO_LINGER 0) gets "$ read error: <strerror> (taken as EOF)" in the reader's
// command log, with its address and [pid/slot] like the other "$" lines, next to
// "$ connection ended before first command". Fails on main: the line does not exist.
func TestReaderReadErrorLogged(t *testing.T) {
	_, spool := spoolStore(t)
	s := NewReaderSite(t, ReaderOptions{Spool: spool, Config: map[string]string{"readerdetaillog": "on"}})

	nc, err := net.DialTimeout("tcp", s.Addr, testkit.Deadline())
	if err != nil {
		t.Fatal(err)
	}
	nc.SetReadDeadline(time.Now().Add(testkit.Deadline()))
	greeting, err := bufio.NewReader(nc).ReadString('\n')
	if err != nil || !strings.HasPrefix(greeting, "200 ") && !strings.HasPrefix(greeting, "201 ") {
		t.Fatalf("greeting: %q %v", greeting, err)
	}
	if err := nc.(*net.TCPConn).SetLinger(0); err != nil {
		t.Fatal(err)
	}
	nc.Close()
	var line string
	testkit.WaitFor(t, 10*time.Second, "a read error line", func() bool {
		for _, l := range strings.Split(generalLog(s), "\n") {
			if strings.Contains(l, "$ read error: ") {
				line = l
				return true
			}
		}
		return false
	})
	if !strings.Contains(line, "(taken as EOF)") || !strings.Contains(line, "127.0.0.1 [") {
		t.Fatalf("read error line %q: want the client address, [pid/slot] and \"(taken as EOF)\"", line)
	}
	t.Logf("%s", line)
}

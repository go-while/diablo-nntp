package e2e

// D-304 diagnostics: a reader connection that ends without any command line (the client
// reads the greeting and closes) is logged as "EOF before first command", next to the
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
// a client that reads the greeting and closes gets "$ EOF before first command" in the
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
	testkit.WaitFor(t, 10*time.Second, "EOF before first command", func() bool {
		return count("$ EOF before first command") >= 1 && count("$ exit articles") >= count("] < QUIT")+1
	})
	time.Sleep(300 * time.Millisecond)
	if n, e, q := count("$ EOF before first command"), count("$ exit articles"), count("] < QUIT"); n != 1 || e != q+1 {
		t.Fatalf("EOF messages %d (want 1), exit lines %d, QUIT commands %d (want exit = QUIT + 1):\n%s", n, e, q, generalLog(s))
	}
}

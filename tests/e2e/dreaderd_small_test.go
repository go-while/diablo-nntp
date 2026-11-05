package e2e

import (
	"context"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	testkit "github.com/go-while/nntp-testkit"
	"github.com/go-while/nntp-testkit/fakeserver"
)

// TestDserverHostsLongLineSkipped (D-94): a dserver.hosts line longer than the read buffer
// is logged with its number and skipped whole. The tail used to be read as a line of its
// own; here it is a complete entry (a host nothing listens on), which must not be set up.
func TestDserverHostsLongLineSkipped(t *testing.T) {
	_, spool := spoolStore(t)
	host, port, _ := net.SplitHostPort(spool.Addr())
	// 3 lines: a long comment-looking line whose tail is an entry, a long entry, a good entry
	long1 := "# " + strings.Repeat(" ", 9000) + "127.0.0.9\ts1p1\n"
	long2 := "127.0.0.8\ts1p1 " + strings.Repeat("junk=1 ", 2000) + "\n"
	hosts := "# first\n" + long1 + long2 + host + "\ts1p" + port + "\n"
	s := NewReaderSite(t, ReaderOptions{Spool: spool, Files: map[string]string{"dserver.hosts": hosts},
		Config: map[string]string{"path_generallog": "%s/general.log"}})
	c := s.Reader(t)
	c.ExpectCmd(211, "GROUP e2e.test") // the good entry after the long lines still works
	c.ExpectCmd(205, "QUIT")
	var log string
	testkit.WaitFor(t, 10*time.Second, "the log lines", func() bool {
		log = generalLog(s)
		return strings.Contains(log, "dserver.hosts: line 2 is longer than") &&
			strings.Contains(log, "dserver.hosts: line 3 is longer than")
	})
	if !regexp.MustCompile(`dserver\.hosts: line 2 is longer than \d+ bytes`).MatchString(log) {
		t.Errorf("log lacks the expected message:\n%s", log)
	}
	if strings.Contains(log, "line 1 is longer") || strings.Contains(log, "line 4 is longer") {
		t.Errorf("a short line was reported as too long:\n%s", log)
	}
	for _, bogus := range []string{"127.0.0.9", "127.0.0.8"} {
		if strings.Contains(log, bogus) {
			t.Errorf("the tail of a long line was read as an entry (%s) in:\n%s", bogus, log)
		}
	}
}

// TestOverWideRangeDoesNotBlock (D-55): a range of two billion article numbers with no
// overview records (the group's high mark is far above the real articles, as it can be in
// xref-slave mode) was walked in one pass, so every other connection of the process waited
// for the whole walk. The walk now takes a bounded number of records per pass: another
// connection is served while it runs, and a narrower wide range still answers correctly.
func TestOverWideRangeDoesNotBlock(t *testing.T) {
	st, spool := spoolStore(t)
	s := NewReaderSite(t, ReaderOptions{Spool: spool})
	id := msgID(t)
	feedReader(t, s, st, "e2e.test", newArticle(id, "e2e.test", "wide range", "body"))

	c := s.Reader(t)
	testkit.WaitFor(t, 10*time.Second, "the article in GROUP", func() bool {
		_, line := c.Cmd("GROUP e2e.test")
		return strings.HasPrefix(line, "211 1")
	})

	// a wide range still answers, with the one article
	s.Run(t, "dgrpctl", "adjust", "e2e.test", "NE", "=20000000")
	c.ExpectCmd(211, "GROUP e2e.test")
	c.ExpectCmd(224, "OVER 1-")
	if got := c.ReadDotLines(); len(got) != 1 || !strings.Contains(got[0], id) {
		t.Errorf("OVER 1- over a wide range: %q, want the one article", got)
	}

	// an enormous one does not hold the process
	s.Run(t, "dgrpctl", "adjust", "e2e.test", "NE", "=2000000000")
	c.ExpectCmd(211, "GROUP e2e.test")
	c.MustSend("OVER 1-\r\n")
	time.Sleep(300 * time.Millisecond) // let the walk start
	start := time.Now()
	c2 := s.Reader(t) // the greeting is part of the wait
	c2.ExpectCmd(211, "GROUP e2e.test")
	if d := time.Since(start); d > 2*time.Second {
		t.Errorf("a second client waited %v for GROUP while OVER walked a wide range", d)
	}
	c2.ExpectCmd(205, "QUIT")
	c.Close() // the walk ends when the client is gone
}

// TestSpoolReplyOverCap (D-101): a line from the spool server longer than the cap (1 MiB)
// is not buffered without end: dreaderd drops the spool connection and logs it, and goes on
// serving other clients.
func TestSpoolReplyOverCap(t *testing.T) {
	huge := strings.Repeat("x", 2<<20)
	spool := fakeserver.New(t, fakeserver.Options{Greeting: "200 fake spool ready",
		Handler: fakeserver.LineHandler(func(_ *fakeserver.Conn, line string) ([]string, bool) {
			switch strings.ToUpper(strings.Fields(line + " x")[0]) {
			case "ARTICLE", "BODY", "HEAD", "STAT":
				return []string{"220 0 " + huge}, false
			case "QUIT":
				return []string{"205 bye"}, true
			}
			return []string{"200 ok"}, false
		})})
	s := NewReaderSite(t, ReaderOptions{Spool: spool,
		Config: map[string]string{"path_generallog": "%s/general.log"}})
	id := msgID(t)
	feedReader(t, s, fakeserver.NewStore(), "e2e.test", newArticle(id, "e2e.test", "over cap", "body"))

	c := s.Reader(t)
	testkit.WaitFor(t, 10*time.Second, "the article in GROUP", func() bool {
		_, line := c.Cmd("GROUP e2e.test")
		return strings.HasPrefix(line, "211 1")
	})
	// dreaderd requeues a request whose server went away to another connection, so the client
	// is not answered while the spool keeps sending the long line (as for a spool that drops
	// the connection); the point is that the line is cut off and logged.
	c.ExpectCmd(223, "STAT 1")
	c.MustSend("BODY 1\r\n")
	testkit.WaitFor(t, 10*time.Second, "the cap in the log", func() bool {
		return strings.Contains(generalLog(s), "MBReadLine line longer than")
	})
	// still alive
	c2 := s.Reader(t)
	c2.ExpectCmd(211, "GROUP e2e.test")
	c2.ExpectCmd(205, "QUIT")
}

// TestDclientIPv6 (D-129): dclient reaches a server on [::1].
func TestDclientIPv6(t *testing.T) {
	dclient := filepath.Join(bin(t), "dclient")
	if _, err := os.Stat(dclient); err != nil {
		t.Skipf("no dclient in %s", bin(t))
	}
	if l, err := net.Listen("tcp6", "[::1]:0"); err != nil {
		t.Skipf("no IPv6 loopback: %v", err)
	} else {
		l.Close()
	}
	srv := fakeserver.New(t, fakeserver.Options{Addr: "[::1]:0", Greeting: "200 fake server ready",
		Handler: fakeserver.LineHandler(func(_ *fakeserver.Conn, line string) ([]string, bool) {
			switch strings.ToUpper(strings.Fields(line + " x")[0]) {
			case "DATE":
				return []string{"111 20261026120000"}, false
			case "QUIT":
				return []string{"205 bye"}, true
			}
			return []string{"500 what?"}, false
		})})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, dclient, "-p", strconv.Itoa(srv.Port()), "::1", "DATE").CombinedOutput()
	if err != nil || !strings.Contains(string(out), "111 20261026120000") {
		t.Errorf("dclient to [::1]: err %v, output %q", err, out)
	}
}

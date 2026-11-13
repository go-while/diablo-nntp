package e2e

import (
	"errors"
	"net"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	testkit "github.com/go-while/nntp-testkit"
	"github.com/go-while/nntp-testkit/client"
)

// TestDeepRevDrcmdSilentClient (L4-06): the main dreaderd process answers drcmd on a unix
// socket from its main loop. It made the accepted connection blocking and read the command
// line with fgets(), so a local client that connected and sent nothing (a hung monitoring
// script) stopped the whole main loop for as long as it stayed: no new client was accepted
// and no connection came back from a reader.
//
// A connection to the control socket that sends nothing, held open: an NNTP client must
// still get its greeting within 15 seconds.
func TestDeepRevDrcmdSilentClient(t *testing.T) {
	_, spool := spoolStore(t)
	s := NewReaderSite(t, ReaderOptions{Spool: spool})

	silent, err := net.DialTimeout("unix", filepath.Join(s.Dir, "run", ".dreadersock"), 10*time.Second)
	if err != nil {
		t.Fatalf("connect to the control socket: %v", err)
	}
	defer silent.Close()
	time.Sleep(500 * time.Millisecond) // the main process has accepted it

	c, err := client.TryDial(s.Addr, 15*time.Second)
	if err != nil {
		t.Fatalf("no greeting for an NNTP client within 15 s while a control connection that sends nothing is open: %v\n"+
			"the main process waits in fgets() for the command line", err)
	}
	c = c.WithTB(t)
	c.ExpectCmd(111, "DATE")
	c.ExpectCmd(205, "QUIT")

	// drcmd itself works as before
	if out := s.Run(t, "drcmd", "status"); out == "" {
		t.Errorf("drcmd status: no output")
	}
}

// TestDeepRevDrcmdDrippingClient: the time a control client gets for its command line is
// counted in all, not per read. A receive timeout per read (SO_RCVTIMEO) let a client that
// sends a byte every 300 ms keep the main process in fgets() for as long as it went on.
//
// A connection to the control socket that sends a byte every 300 ms for 12 s and no newline:
// an NNTP client must still get its greeting within 6 seconds, and the control connection
// must be closed within 5 seconds.
func TestDeepRevDrcmdDrippingClient(t *testing.T) {
	_, spool := spoolStore(t)
	s := NewReaderSite(t, ReaderOptions{Spool: spool})

	drip, err := net.DialTimeout("unix", filepath.Join(s.Dir, "run", ".dreadersock"), 10*time.Second)
	if err != nil {
		t.Fatalf("connect to the control socket: %v", err)
	}
	defer drip.Close()
	start := time.Now()
	closed := make(chan time.Duration, 1)
	go func() {
		for time.Since(start) < 12*time.Second {
			drip.SetWriteDeadline(time.Now().Add(time.Second))
			if _, err := drip.Write([]byte("s")); err != nil {
				closed <- time.Since(start)
				return
			}
			time.Sleep(300 * time.Millisecond)
		}
		closed <- 0
	}()
	time.Sleep(500 * time.Millisecond) // the main process has accepted it and reads

	c, err := client.TryDial(s.Addr, 6*time.Second)
	if err != nil {
		t.Errorf("no greeting for an NNTP client within 6 s while a control client sends its line a byte at a time: %v", err)
	} else {
		c = c.WithTB(t)
		c.ExpectCmd(205, "QUIT")
	}
	if d := <-closed; d == 0 || d > 5*time.Second {
		t.Errorf("the control connection that never finished its line was closed after %v (0: not at all in 12 s), want within 5 s", d)
	}
	if out := s.Run(t, "drcmd", "status"); out == "" {
		t.Errorf("drcmd status: no output")
	}
}

// drcmdSend sends raw on a new control connection, closes the sending side and returns what
// came back and whether the connection was closed (EOF or reset, not the read deadline). For
// a line that is to be refused (refused), dreaderd closes the connection as soon as the byte
// past the buffer shows the line too long, so the write may fail with EPIPE or ECONNRESET;
// then what follows is still read, and broke tells that the write broke off. The count of the
// failed write is not used: NetBSD (and other BSD kernels) report -1 and EPIPE for a write()
// that sent most of its bytes before the close, and Go then counts 0 bytes, so that a count of
// drcmdMaxLine or more cannot be asked for. That the refusal came from the length of the
// line is what the callers check in the log. For any other line the whole write must go through.
// sndbuf > 0 sets the client's send buffer (a small one makes the write break off).
func drcmdSend(t *testing.T, sock, raw string, refused bool, sndbuf int) (text string, closed, broke bool) {
	t.Helper()
	c, err := net.DialTimeout("unix", sock, 10*time.Second)
	if err != nil {
		t.Fatalf("connect to the control socket: %v", err)
	}
	defer c.Close()
	if sndbuf > 0 {
		if err := c.(*net.UnixConn).SetWriteBuffer(sndbuf); err != nil {
			t.Fatal(err)
		}
	}
	c.SetWriteDeadline(time.Now().Add(5 * time.Second))
	if n, err := c.Write([]byte(raw)); err != nil {
		if refused && (errors.Is(err, syscall.EPIPE) || errors.Is(err, syscall.ECONNRESET)) {
			text, closed = readToEOF(c, 8*time.Second)
			return text, closed, true
		}
		t.Fatalf("write (%d of %d bytes): %v", n, len(raw), err)
	}
	c.(*net.UnixConn).CloseWrite()
	text, closed = readToEOF(c, 8*time.Second)
	return text, closed, false
}

// drcmdMaxLine is the buffer of a control command line, its NUL included (MAXLINE, lib/defs.h).
const drcmdMaxLine = 16384

// TestDeepRevDrcmdLongLineAndEOF: a control command line that does not fit into the buffer
// (MAXLINE) is not run: fgets() ran its first part as a command, and so did the first
// version of the deadline. A last line without a newline at EOF is still run, as fgets() did.
func TestDeepRevDrcmdLongLineAndEOF(t *testing.T) {
	_, spool := spoolStore(t)
	s := NewReaderSite(t, ReaderOptions{Spool: spool})
	sock := filepath.Join(s.Dir, "run", ".dreadersock")

	t.Run("longer than the buffer", func(t *testing.T) {
		text, eof, _ := drcmdSend(t, sock, "status "+strings.Repeat("x", 20000), true, 0)
		if !eof || text != "" {
			t.Errorf("a command line of 20007 bytes without a newline: got %q (closed: %v), want nothing and the close", text, eof)
		}
		if !strings.Contains(generalLog(s), "drcmd: command line longer than") {
			t.Errorf("the refused long line is not in the log")
		}
	})
	// a send buffer of 2 KB and 200000 bytes: the write cannot be done when dreaderd closes,
	// so this takes the branch for a write that broke off (as on FreeBSD), on any system
	t.Run("longer than the buffer, the write breaks off", func(t *testing.T) {
		refusals := strings.Count(generalLog(s), "drcmd: command line longer than")
		text, closed, broke := drcmdSend(t, sock, "status "+strings.Repeat("x", 200000), true, 2048)
		if !broke {
			t.Fatalf("the write of 200000 bytes with a 2 KB send buffer went through: the branch for a broken-off write was not taken")
		}
		if !closed || text != "" {
			t.Errorf("a command line of 200007 bytes, write broken off: got %q (closed: %v), want nothing and the close", text, closed)
		}
		testkit.WaitFor(t, 5*time.Second, "the refusal in the log", func() bool {
			return strings.Count(generalLog(s), "drcmd: command line longer than") > refusals
		})
	})
	t.Run("last line at EOF", func(t *testing.T) {
		if text, _, _ := drcmdSend(t, sock, "status", false, 0); !strings.Contains(text, "Connect=") {
			t.Errorf("\"status\" without a newline, then EOF: got %q, want the status line", text)
		}
	})
}

// TestDeepRevDrcmdLineBoundary: the buffer of a control command line is MAXLINE (16384) bytes
// with its NUL. A line whose text is 16383 bytes fits: at EOF without a newline and with a
// newline it is run (fgets() ran it too); one byte more is refused.
func TestDeepRevDrcmdLineBoundary(t *testing.T) {
	const maxline = drcmdMaxLine
	_, spool := spoolStore(t)
	s := NewReaderSite(t, ReaderOptions{Spool: spool})
	sock := filepath.Join(s.Dir, "run", ".dreadersock")
	line := func(n int) string { return "status " + strings.Repeat("x", n-len("status ")) }
	for _, tc := range []struct {
		name, raw string
		run       bool
	}{
		{"16383 bytes at EOF", line(maxline - 1), true},
		{"16383 bytes and a newline", line(maxline-1) + "\n", true},
		{"16382 bytes and a newline", line(maxline-2) + "\n", true},
		{"16384 bytes at EOF", line(maxline), false},
		{"16384 bytes and a newline", line(maxline) + "\n", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			refusals := strings.Count(generalLog(s), "drcmd: command line longer than")
			text, closed, _ := drcmdSend(t, sock, tc.raw, !tc.run, 0)
			if ran := strings.Contains(text, "Connect="); ran != tc.run {
				t.Errorf("%s: answer %q, run %v, want %v", tc.name, text, ran, tc.run)
			}
			if !tc.run {
				if !closed {
					t.Errorf("%s: the connection was not closed", tc.name)
				}
				testkit.WaitFor(t, 5*time.Second, "the refusal in the log", func() bool {
					return strings.Count(generalLog(s), "drcmd: command line longer than") > refusals
				})
			}
		})
	}
}

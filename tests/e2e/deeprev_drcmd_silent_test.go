package e2e

import (
	"net"
	"path/filepath"
	"strings"
	"testing"
	"time"

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

// TestDeepRevDrcmdLongLineAndEOF: a control command line that does not fit into the buffer
// (MAXLINE) is not run: fgets() ran its first part as a command, and so did the first
// version of the deadline. A last line without a newline at EOF is still run, as fgets() did.
func TestDeepRevDrcmdLongLineAndEOF(t *testing.T) {
	_, spool := spoolStore(t)
	s := NewReaderSite(t, ReaderOptions{Spool: spool})
	sock := filepath.Join(s.Dir, "run", ".dreadersock")
	send := func(t *testing.T, raw string) (string, bool) {
		t.Helper()
		c, err := net.DialTimeout("unix", sock, 10*time.Second)
		if err != nil {
			t.Fatalf("connect to the control socket: %v", err)
		}
		defer c.Close()
		c.SetWriteDeadline(time.Now().Add(5 * time.Second))
		if _, err := c.Write([]byte(raw)); err != nil {
			t.Fatalf("write: %v", err)
		}
		c.(*net.UnixConn).CloseWrite()
		return readToEOF(c, 8*time.Second)
	}

	t.Run("longer than the buffer", func(t *testing.T) {
		text, eof := send(t, "status "+strings.Repeat("x", 20000))
		if !eof || text != "" {
			t.Errorf("a command line of 20007 bytes without a newline: got %q (closed: %v), want nothing and the close", text, eof)
		}
		if !strings.Contains(generalLog(s), "drcmd: command line longer than") {
			t.Errorf("the refused long line is not in the log")
		}
	})
	t.Run("last line at EOF", func(t *testing.T) {
		if text, _ := send(t, "status"); !strings.Contains(text, "Connect=") {
			t.Errorf("\"status\" without a newline, then EOF: got %q, want the status line", text)
		}
	})
}

// TestDeepRevDrcmdLineBoundary: the buffer of a control command line is MAXLINE (16384) bytes
// with its NUL. A line whose text is 16383 bytes fits: at EOF without a newline and with a
// newline it is run (fgets() ran it too); one byte more is refused.
func TestDeepRevDrcmdLineBoundary(t *testing.T) {
	const maxline = 16384 // lib/defs.h
	_, spool := spoolStore(t)
	s := NewReaderSite(t, ReaderOptions{Spool: spool})
	sock := filepath.Join(s.Dir, "run", ".dreadersock")
	send := func(t *testing.T, raw string) string {
		t.Helper()
		c, err := net.DialTimeout("unix", sock, 10*time.Second)
		if err != nil {
			t.Fatalf("connect to the control socket: %v", err)
		}
		defer c.Close()
		c.SetWriteDeadline(time.Now().Add(5 * time.Second))
		if _, err := c.Write([]byte(raw)); err != nil {
			t.Fatalf("write: %v", err)
		}
		c.(*net.UnixConn).CloseWrite()
		text, _ := readToEOF(c, 8*time.Second)
		return text
	}
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
			text := send(t, tc.raw)
			if ran := strings.Contains(text, "Connect="); ran != tc.run {
				t.Errorf("%s: answer %q, run %v, want %v", tc.name, text, ran, tc.run)
			}
		})
	}
}

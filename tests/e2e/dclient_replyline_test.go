package e2e

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

type replyBoundaryOutput struct {
	buf       bytes.Buffer
	threshold int
	seen      chan struct{}
}

func (b *replyBoundaryOutput) Write(p []byte) (int, error) {
	n, err := b.buf.Write(p)
	if b.threshold > 0 && b.buf.Len() >= b.threshold {
		close(b.seen)
		b.threshold = 0
	}
	return n, err
}

// Reply lines can span fgets pieces; a dot in a later piece is data, not
// the multiline terminator. The next command must get its own reply.
func TestDclientReplyLineBoundaries(t *testing.T) {
	// piece is what main's fgets(line, MAXLINE) read at once. chunk is what
	// ReadReplyLine prints at once (its buffer is MAXLINE bytes).
	const piece = 16383
	const chunk = 16384
	const greeting = "200 ready\r\n"
	const quit = "205 bye\r\n"
	const date = "111 20261121000000\r\n"
	for _, tc := range []struct {
		name, greeting, command, reply, quit string
	}{
		{"single-long", greeting, "DATE", "111 " + strings.Repeat("s", 2*piece) + "\r\n", quit},
		{"single-split-crlf", greeting, "DATE", "111 " + strings.Repeat("c", piece-5) + "\r\n", quit},
		{"data-dot-at-piece", greeting, "HELP",
			"100 help follows\r\n" + strings.Repeat("d", piece) + ".\r\nstill in help\r\n.\r\n", quit},
		{"data-split-crlf", greeting, "HELP",
			"100 help follows\r\n" + strings.Repeat("e", piece-1) + "\r\nstill in help\r\n.\r\n", quit},
		// Edges of ReadReplyLine's own chunk (the two split-crlf cases above
		// already have CR as byte 16383 and LF as byte 16384, the end of
		// the chunk): CR is the last byte of the chunk and LF the first of
		// the next; CR and LF both in the next chunk.
		{"single-cr-last-of-chunk", greeting, "DATE", "111 " + strings.Repeat("a", chunk-5) + "\r\n", quit},
		{"single-crlf-in-next-chunk", greeting, "DATE", "111 " + strings.Repeat("b", chunk-4) + "\r\n", quit},
		{"data-cr-last-of-chunk", greeting, "HELP",
			"100 help follows\r\n" + strings.Repeat("f", chunk-1) + "\r\nstill in help\r\n.\r\n", quit},
		// A dot as the first byte of the second chunk of a data line is data.
		// Main's fgets pieces do not put it first in a piece, so this one is
		// a guard of the chunk handling, not a base reproducer.
		{"data-dot-at-chunk-start", greeting, "HELP",
			"100 help follows\r\n" + strings.Repeat("i", chunk) + ".\r\nstill in help\r\n.\r\n", quit},
		// A dot at the start of main's third fgets piece (2 x 16383 bytes in).
		{"data-dot-at-third-piece", greeting, "HELP",
			"100 help follows\r\n" + strings.Repeat("j", 2*piece) + ".\r\nstill in help\r\n.\r\n", quit},
		{"data-nul", greeting, "HELP", "100 help follows\r\nbefore\x00after\r\n.\r\n", quit},
		{"greeting-512", "200 " + strings.Repeat("g", 506) + "\r\n", "DATE", date, quit},
		{"greeting-long", "200 " + strings.Repeat("g", 2*piece) + "\r\n", "DATE", date, quit},
		{"quit-512", greeting, "DATE", date, "205 " + strings.Repeat("q", 506) + "\r\n"},
		{"quit-long", greeting, "DATE", date, "205 " + strings.Repeat("q", 2*piece) + "\r\n"},
		// The same long fragment without a line end; only the end of the
		// connection differs: FIN (accepted) or RST (a read error).
		{"quit-clean-eof", greeting, "DATE", date, "205 " + strings.Repeat("q", 2*piece)},
		{"quit-read-error", greeting, "DATE", date, "205 " + strings.Repeat("q", 2*piece)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ln, err := net.ListenTCP("tcp", &net.TCPAddr{IP: net.ParseIP("127.0.0.1")})
			if err != nil {
				t.Fatal(err)
			}
			defer ln.Close()
			if err := ln.SetDeadline(time.Now().Add(10 * time.Second)); err != nil {
				t.Fatal(err)
			}
			server := make(chan error, 1)
			var out replyBoundaryOutput
			abortQuit := tc.name == "quit-read-error"
			if abortQuit {
				out.threshold = len(tc.greeting) + len(">>STDIN\r\n") + len(tc.reply) + len(date) + 1
				out.seen = make(chan struct{})
			}
			go func() {
				c, err := ln.AcceptTCP()
				if err != nil {
					server <- err
					return
				}
				defer c.Close()
				if err := c.SetDeadline(time.Now().Add(10 * time.Second)); err != nil {
					server <- err
					return
				}
				if _, err := io.WriteString(c, tc.greeting); err != nil {
					server <- err
					return
				}
				r := bufio.NewReader(c)
				for _, step := range []struct{ command, reply string }{
					{tc.command, tc.reply},
					{"DATE", date},
					{"QUIT", tc.quit},
				} {
					line, err := r.ReadString('\n')
					if err != nil {
						server <- fmt.Errorf("waiting for %s: %w", step.command, err)
						return
					}
					if line != step.command+"\r\n" {
						server <- fmt.Errorf("got command %q, want %q", line, step.command)
						return
					}
					if _, err := io.WriteString(c, step.reply); err != nil {
						server <- err
						return
					}
				}
				if abortQuit {
					// Reset after dclient has started printing this reply, not
					// while its bytes may still be unread in the socket.
					select {
					case <-out.seen:
					case <-time.After(10 * time.Second):
						server <- fmt.Errorf("dclient did not print the QUIT reply prefix")
						return
					}
					// Let dclient's TCP stack acknowledge the reply first. On
					// FreeBSD (seen on 15.1) a reset sent before the delayed ACK
					// (100 ms) is ignored, the answer is a challenge ACK, and the
					// reset the server's kernel owes to that ACK can go missing
					// while the machine sends many closed-port resets (probably
					// the reset rate limit). dclient has no read timeout and then
					// waits until the test's 15 s run out. 300 ms is three
					// delayed-ACK times there; the wait is time based, the server
					// has no portable way to see the ACK.
					time.Sleep(300 * time.Millisecond)
					server <- c.SetLinger(0)
					return
				}
				server <- nil
			}()
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, filepath.Join(bin(t), "dclient"),
				"-v", "-p", strconv.Itoa(ln.Addr().(*net.TCPAddr).Port), "127.0.0.1")
			cmd.Stdin = strings.NewReader(tc.command + "\nDATE\n")
			cmd.WaitDelay = time.Second
			cmd.Stdout, cmd.Stderr = &out, &out
			runErr := cmd.Run()
			serverErr := <-server
			want := tc.greeting + ">>STDIN\r\n" + tc.reply + date + tc.quit
			wantExit := 0
			if abortQuit {
				wantExit = 2
				// The reset can discard bytes still in flight.
				want = tc.greeting + ">>STDIN\r\n" + tc.reply + date + "205 "
			}
			if cmd.ProcessState == nil || cmd.ProcessState.ExitCode() != wantExit ||
				serverErr != nil || !strings.HasPrefix(out.buf.String(), want) {
				i, got, expected := diffAt(out.buf.String(), want)
				t.Errorf("dclient lost reply-line boundaries: process %v; server %v; output %d bytes, expected prefix %d bytes; first difference at %d: got %s, want %s",
					runErr, serverErr, out.buf.Len(), len(want), i, got, expected)
			}
		})
	}
}

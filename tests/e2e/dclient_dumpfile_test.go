package e2e

// dclient -f: the lines of a dump file are sent as they are, each ending in CR LF.

import (
	"bytes"
	"context"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// dumpSent runs "dclient -f" on a dump file against a raw listener that sends the greeting,
// collects the bytes up to the QUIT line and answers 205; it returns what the server got.
func dumpSent(t *testing.T, file string) string {
	t.Helper()
	dclient := filepath.Join(bin(t), "dclient")
	if _, err := os.Stat(dclient); err != nil {
		t.Skipf("no dclient in %s", bin(t))
	}
	dump := filepath.Join(t.TempDir(), "dump.txt")
	if err := os.WriteFile(dump, []byte(file), 0o644); err != nil {
		t.Fatal(err)
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	got := make(chan []byte, 1)
	go func() {
		c, err := ln.Accept()
		if err != nil {
			got <- nil
			return
		}
		defer c.Close()
		c.SetDeadline(time.Now().Add(20 * time.Second))
		c.Write([]byte("200 fake server ready\r\n"))
		var buf bytes.Buffer
		b := make([]byte, 4096)
		for !bytes.HasSuffix(buf.Bytes(), []byte("QUIT\r\n")) {
			n, err := c.Read(b)
			buf.Write(b[:n])
			if err != nil {
				break
			}
		}
		c.Write([]byte("205 bye\r\n"))
		got <- buf.Bytes()
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, dclient, "-f", dump, "-p", strconv.Itoa(ln.Addr().(*net.TCPAddr).Port), "127.0.0.1").CombinedOutput()
	if err != nil {
		t.Fatalf("dclient -f: %v\n%s", err, out)
	}
	return string(<-got)
}

func diffAt(g, want string) (int, string, string) {
	i := 0
	for i < len(g) && i < len(want) && g[i] == want[i] {
		i++
	}
	lo := func(s string) string {
		a, b := max(i-20, 0), min(i+20, len(s))
		return strconv.Quote(s[a:b])
	}
	return i, lo(g), lo(want)
}

// TestDclientDumpFileLines: a dump file with a blank line, a line with CR LF, a line longer than
// 16383 bytes (the most fgets reads into the 16384-byte line buffer; also ones that split at the
// piece edge, with the CR LF split too) and a
// last line without a newline reaches the server whole: no byte is cut, no CR LF is added inside
// a long line, each line ends in CR LF, and the blank line stays blank. Fails on main: dclient
// cut the last byte of the last line and the last byte of each full piece of the long line, and
// added a CR LF after each piece.
func TestDclientDumpFileLines(t *testing.T) {
	const piece = 16383 // the buffer holds 16384 bytes, fgets reads at most 16383
	long := strings.Repeat("abcdefghi", 2223)[:20000]
	edgeLF := strings.Repeat("e", piece) + "\n"        // exactly one full piece, then LF
	edgeCRLF := strings.Repeat("f", piece-1) + "\r\n"  // CR is the last byte of the piece
	edgeCRX := strings.Repeat("g", piece-1) + "\rhi\n" // CR at the end of the piece is not part of a CR LF
	file := "alpha\n\ncrlf\r\n" + long + "\n" + edgeLF + edgeCRLF + edgeCRX + "tail"
	want := "alpha\r\n\r\ncrlf\r\n" + long + "\r\n" +
		strings.Repeat("e", piece) + "\r\n" +
		strings.Repeat("f", piece-1) + "\r\n" +
		strings.Repeat("g", piece-1) + "\rhi\r\n" +
		"tail\r\n" + "QUIT\r\n"
	if g := dumpSent(t, file); g != want {
		i, a, b := diffAt(g, want)
		t.Errorf("dclient -f sent %d bytes, want %d; first difference at byte %d: got %s, want %s", len(g), len(want), i, a, b)
	}
}

// TestDclientDumpFileLastLineSizes: a file that is one line without a final newline, of 16383
// bytes (exactly one fgets read), 16384, 16385, 2 x 16383 and 2 x 16383 + 1 bytes, or one
// that ends in a CR, is sent whole and ended by CR LF, so the QUIT that follows is a line of its
// own. Fails on main: the last byte was cut. A last line of exactly 16383 (or 2 x 16383) bytes
// must still be ended by CR LF, or QUIT would follow on the same line.
func TestDclientDumpFileLastLineSizes(t *testing.T) {
	for _, tc := range []struct {
		name, line string
	}{
		{"16383", strings.Repeat("a", 16383)},
		{"16384", strings.Repeat("b", 16384)},
		{"16385", strings.Repeat("c", 16385)},
		{"2x16383", strings.Repeat("d", 2*16383)},
		{"2x16383+1", strings.Repeat("e", 2*16383+1)},
		{"16383 ending in CR", strings.Repeat("f", 16382) + "\r"},
		{"16384 ending in CR", strings.Repeat("g", 16383) + "\r"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			want := tc.line + "\r\nQUIT\r\n"
			if g := dumpSent(t, tc.line); g != want {
				i, a, b := diffAt(g, want)
				t.Errorf("dclient -f sent %d bytes, want %d; first difference at byte %d: got %s, want %s", len(g), len(want), i, a, b)
			}
		})
	}
}

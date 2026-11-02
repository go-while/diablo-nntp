package e2e

// A dserver.hosts entry with localspool=<dir> makes dreaderd ask the spool
// server only "whereis <msgid>" and then read the article itself from the
// file it names (dreaderd/dfa.c, NNSLA_Loop).  None of the other tests take
// that path: their fake spool serves article text over NNTP.
//
// TestReaderLocalSpoolBody fetches a large article through it, across many
// read-buffer boundaries.  TestReaderLocalSpoolShortFile has the spool name
// more bytes than the file holds -- what history and the spool look like
// after a crash, since the spool is not fsync'ed.  NewDFA's lseek() check
// let such a file through; the client got "222", part of the body, and then
// nothing.  Now the file is refused before the response starts.

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// localSpoolServer answers "whereis <id>" with the file <dir>/<id>.art,
// claiming extra bytes beyond its real size.
func localSpoolServer(t *testing.T, dir string, extra int64) net.Listener {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { l.Close() })
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				fmt.Fprintf(c, "200 localspool e2e\r\n")
				r := bufio.NewReader(c)
				for {
					ln, err := r.ReadString('\n')
					if err != nil {
						return
					}
					f := strings.Fields(ln)
					switch {
					case len(f) >= 2 && strings.EqualFold(f[0], "whereis"):
						name := strings.Trim(f[1], "<>") + ".art"
						fi, err := os.Stat(filepath.Join(dir, name))
						if err != nil {
							fmt.Fprintf(c, "430 no such article\r\n")
							continue
						}
						fmt.Fprintf(c, "223 %s in %s offset 0 length %d\r\n",
							f[1], name, fi.Size()+extra)
					case len(f) > 0 && strings.EqualFold(f[0], "quit"):
						fmt.Fprintf(c, "205 bye\r\n")
						return
					default:
						fmt.Fprintf(c, "430 no such article\r\n")
					}
				}
			}(c)
		}
	}()
	return l
}

// localSpoolSite starts a reader whose only spool is a localspool= entry,
// feeds it one article of n body lines and stores that article in the spool
// directory.  It returns the site, the message-id and the body.
func localSpoolSite(t *testing.T, extra int64, n int) (*Site, string, []string) {
	t.Helper()
	dir := t.TempDir()
	l := localSpoolServer(t, dir, extra)
	_, port, _ := net.SplitHostPort(l.Addr().String())
	s := NewReaderSite(t, ReaderOptions{SpoolAddr: l.Addr().String(), NoStart: true,
		Files: map[string]string{"dserver.hosts": "127.0.0.1\ts1p" + port + " localspool=" + dir + "\n"}})
	startReader(t, s, "[127.0.0.1]:"+strings.Split(s.Addr, ":")[1])

	id := msgID(t)
	body := make([]string, n)
	for i := range body {
		body[i] = fmt.Sprintf("%06d %s", i, strings.Repeat("x", 68))
	}
	art := newArticle(id, "e2e.test", "localspool", body...)
	if err := os.WriteFile(filepath.Join(dir, strings.Trim(id, "<>")+".art"),
		[]byte(strings.Join(art, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	p := s.dialFrom(t, PeerIP)
	expect(t, p, 335, "IHAVE %s", id)
	send(t, p, wire(art))
	status(t, p, 235, "IHAVE")
	return s, id, body
}

func TestReaderLocalSpoolBody(t *testing.T) {
	s, id, body := localSpoolSite(t, 0, 14000) // ~1 MB, many read() buffers
	c := s.Reader(t)
	expect(t, c, 222, "BODY %s", id)
	got := c.ReadDotLines()
	if len(got) != len(body) {
		t.Fatalf("BODY via localspool: got %d lines, want %d", len(got), len(body))
	}
	for i := range body {
		if got[i] != body[i] {
			t.Fatalf("BODY via localspool: line %d = %q, want %q", i, got[i], body[i])
		}
	}
	expect(t, c, 205, "QUIT")
}

func TestReaderLocalSpoolShortFile(t *testing.T) {
	s, id, _ := localSpoolSite(t, 4096, 50)
	c := s.Reader(t)
	// NewDFA refuses the file before any of the response is sent, and the
	// reader falls back to "article" on the spool server, which (here) does
	// not have it.  A 222 means the short file was served.
	if code, line := c.Cmd("BODY %s", id); code != 430 {
		t.Fatalf("BODY of a short spool file: %q, want 430", line)
	}
	// The connection is still usable afterwards.
	expect(t, c, 205, "QUIT")
}

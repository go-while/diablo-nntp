package e2e

import (
	"bufio"
	"net"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// dsyncPeer is the remote end of one dsyncgroups run. It answers 200 to the greeting and to
// MODE READER, the lines of Active after a 215 to LIST ACTIVE and those of Newsgroups to
// LIST NEWSGROUPS (each ending with "."), and 205 to QUIT. With NoQuitReply it closes the
// connection instead of answering QUIT. It returns the port it listens on.
type dsyncPeer struct {
	Active      []string
	Newsgroups  []string
	NoQuitReply bool
}

func (p dsyncPeer) listen(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		defer c.Close()
		c.Write([]byte("200 fake peer ready\r\n"))
		r := bufio.NewReader(c)
		for {
			line, err := r.ReadString('\n')
			if err != nil {
				return
			}
			switch strings.ToUpper(strings.Fields(line + " x")[0]) {
			case "MODE":
				c.Write([]byte("200 reader ok\r\n"))
			case "LIST":
				lines := p.Active
				if strings.HasPrefix(strings.ToUpper(line), "LIST NEWSGROUPS") {
					lines = p.Newsgroups
				}
				c.Write([]byte("215 list follows\r\n"))
				for _, l := range lines {
					c.Write([]byte(l + "\r\n"))
				}
				c.Write([]byte(".\r\n"))
			case "QUIT":
				if !p.NoQuitReply {
					c.Write([]byte("205 bye\r\n"))
				}
				return
			default:
				c.Write([]byte("500 what?\r\n"))
			}
		}
	}()
	return ln.Addr().(*net.TCPAddr).Port
}

// TestDsyncgroupsBlankNewsgroupLine (D-109): with -w, a blank LIST NEWSGROUPS line gave
// WildCmp() a NULL group name from strtok() and crashed dsyncgroups. A blank line is skipped
// and the next group is still read.
func TestDsyncgroupsBlankNewsgroupLine(t *testing.T) {
	s := NewSite(t, SiteOptions{NoStart: true})
	port := dsyncPeer{
		Active:     []string{"alpha 10 1 y"},
		Newsgroups: []string{"", "alpha desc of alpha"},
	}.listen(t)
	out := s.Run(t, "dsyncgroups", "-h", "127.0.0.1", "-p", strconv.Itoa(port),
		"-F", filepath.Join(s.Dir, "dactive.kp"), "-i", "-w", "a*", "-z")
	if !strings.Contains(out, "1 groups returned by list newsgroups") {
		t.Errorf("want 1 newsgroup (alpha) from list newsgroups in:\n%s", out)
	}
}

// TestDsyncgroupsPeerClosesAtQuit (D-109): when the peer closes the connection instead of
// answering QUIT, CommandResponse() returns 0 without setting its reply pointer, and
// dsyncgroups printed the caller's uninitialised pointer: a crash, or (with the stack slot of
// an earlier reply reused) that earlier reply. The fix reports the error with an empty reply
// and exits 1, so nothing may follow the colon.
func TestDsyncgroupsPeerClosesAtQuit(t *testing.T) {
	s := NewSite(t, SiteOptions{NoStart: true})
	port := dsyncPeer{Active: []string{"alpha 10 1 y"}, NoQuitReply: true}.listen(t)
	out, err := s.TryRun("dsyncgroups", "-h", "127.0.0.1", "-p", strconv.Itoa(port),
		"-F", filepath.Join(s.Dir, "dactive.kp"), "-z")
	if err == nil {
		t.Errorf("dsyncgroups succeeded although the peer closed at QUIT:\n%s", out)
	}
	const label = "error sending quit command:"
	i := strings.Index(out, label)
	if i < 0 {
		t.Fatalf("no error message for the missing QUIT reply in:\n%s", out)
	}
	rest := out[i+len(label):]
	if j := strings.IndexByte(rest, '\n'); j >= 0 {
		rest = rest[:j]
	}
	if strings.TrimSpace(rest) != "" {
		t.Errorf("the missing QUIT reply was printed as %q (an unset reply pointer), want nothing", rest)
	}
}

// TestDsyncgroupsShortActiveLine (D-109): a blank or short LIST ACTIVE line ("", "alpha",
// "alpha 10") left the group name or an article number NULL after strtok() and dsyncgroups
// dereferenced it (with -w, WildCmp() read the NULL name). Such lines are skipped, with and
// without -w, and the complete line after them is still read.
func TestDsyncgroupsShortActiveLine(t *testing.T) {
	for _, wild := range []string{"", "a*"} {
		wild := wild
		t.Run("wild="+wild, func(t *testing.T) {
			s := NewSite(t, SiteOptions{NoStart: true})
			port := dsyncPeer{
				Active: []string{"", "alpha", "alpha 10", "alpha 10 1 y"},
			}.listen(t)
			args := []string{"-h", "127.0.0.1", "-p", strconv.Itoa(port),
				"-F", filepath.Join(s.Dir, "dactive.kp"), "-z"}
			if wild != "" {
				args = append(args, "-w", wild)
			}
			out, err := s.TryRun("dsyncgroups", args...)
			if err != nil {
				t.Errorf("dsyncgroups failed (%v) on short LIST ACTIVE lines:\n%s", err, out)
			}
			if !strings.Contains(out, "1 groups returned by list active") {
				t.Errorf("want 1 group (alpha) from list active in:\n%s", out)
			}
		})
	}
}

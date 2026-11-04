package e2e

import (
	"bufio"
	"encoding/binary"
	"errors"
	"net"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// TestReaderAccessLongLineRefused: dreader.access was read with fgets(buf[1024]); a longer line
// was split and its tail parsed as a line of its own (here a clustername directive). Such a
// line is now refused with its line number, and the file is not installed. A long comment
// line is skipped whole.
func TestReaderAccessLongLineRefused(t *testing.T) {
	long := "welcome " + strings.Repeat("w", 1015) + "clustername tail.example"
	s := NewReaderSite(t, ReaderOptions{SpoolAddr: "127.0.0.1:1", NoStart: true,
		Access: vserverWith(long),
		Config: map[string]string{"path_generallog": "%s/general.log"}})
	startReaderNoWait(t, s)
	var log string
	waitFor(t, 5*time.Second, "the refusal in the log", func() bool {
		log = generalLog(s)
		return strings.Contains(log, "line 2 is longer than")
	})
	if !strings.Contains(log, "not updating") {
		t.Errorf("dreader.access with the long line was not refused:\n%s", log)
	}
}

// TestReaderAccessLongCommentIgnored: a comment line longer than the read buffer is not an
// error, and its tail (which here looks like a directive) is not parsed.
func TestReaderAccessLongCommentIgnored(t *testing.T) {
	s := NewReaderSite(t, ReaderOptions{SpoolAddr: "127.0.0.1:1",
		Access: "# " + strings.Repeat("c", 1021) + "bogusdirective x\n" + DefaultReaderAccess,
		Config: map[string]string{"path_generallog": "%s/general.log"}})
	if g := readerGreeting(s.Addr); !isService(g) {
		t.Errorf("no service with a long comment line: %q", g)
	}
	if log := generalLog(s); strings.Contains(log, "longer than") {
		t.Errorf("a long comment line was refused:\n%s", log)
	}
}

// TestDsyncgroupsLongCredentialsRefused: the authinfo command was built in a 4096-byte buffer,
// so a longer password was cut and the cut one sent. dsyncgroups now stops before sending it.
func TestDsyncgroupsLongCredentialsRefused(t *testing.T) {
	s := NewSite(t, SiteOptions{NoStart: true})
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	got := make(chan string, 8)
	go func() {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		defer c.Close()
		c.Write([]byte("200 ready\r\n"))
		r := bufio.NewReader(c)
		for {
			line, err := r.ReadString('\n')
			if err != nil {
				return
			}
			up := strings.ToUpper(line)
			if strings.HasPrefix(up, "AUTHINFO PASS") {
				got <- strconv.Itoa(len(strings.TrimSpace(line)))
			}
			switch {
			case strings.HasPrefix(up, "AUTHINFO USER"):
				c.Write([]byte("381 pass\r\n"))
			case strings.HasPrefix(up, "AUTHINFO PASS"):
				c.Write([]byte("281 ok\r\n"))
			case strings.HasPrefix(up, "MODE"):
				c.Write([]byte("200 ok\r\n"))
			case strings.HasPrefix(up, "QUIT"):
				c.Write([]byte("205 bye\r\n"))
				return
			default:
				c.Write([]byte("500 what\r\n"))
			}
		}
	}()
	port := ln.Addr().(*net.TCPAddr).Port
	out, err := s.TryRun("dsyncgroups", "-h", "127.0.0.1", "-p", strconv.Itoa(port),
		"-F", filepath.Join(s.Dir, "dactive.kp"), "-U", "fred", "-u", strings.Repeat("p", 5000), "-z")
	if err == nil {
		t.Errorf("dsyncgroups succeeded with a 5000-byte password:\n%s", out)
	}
	select {
	case n := <-got:
		t.Errorf("a cut AUTHINFO PASS line of %s bytes was sent:\n%s", n, out)
	default:
	}
	if !strings.Contains(out, "password is too long") {
		t.Errorf("no message that the password is too long:\n%s", out)
	}
}

// TestOverGzipTruncatedHeader: a data file .gz whose header claims an extra field longer than
// the file made dreaderd and dexpireover step past the end of the mapping. dexpireover -R
// now reports the header as bad and does not crash.
func TestOverGzipTruncatedHeader(t *testing.T) {
	if !builtWithZlib(t) {
		t.Skip("no zlib")
	}
	s, home := overSite(t, map[string]string{"readergrouphash": "hierarchy"})
	s.Run(t, "dgrpctl", "adjust", "e2e.test", "NE", "=1")
	head := overHead("e2e.test")
	index := append(head, make([]byte, 1000*32)...)
	slot := index[len(head)+32:]
	o := binary.NativeEndian
	o.PutUint32(slot[0:], 1)
	o.PutUint32(slot[8:], 10)
	o.PutUint32(slot[12:], 0x952a1fae)
	o.PutUint32(slot[16:], 0x54dcbdde)
	o.PutUint32(slot[20:], 1000)
	o.PutUint32(slot[24:], uint32(time.Now().Unix()))
	writeOver(t, filepath.Join(home, "e2e", "o.0.e2e.test"), index)

	// magic, deflate, FEXTRA, mtime, xfl, os, XLEN 0xFFF0, 12 bytes, ISIZE 1000
	gz := []byte{0x1f, 0x8b, 8, 4, 0, 0, 0, 0, 0, 3, 0xF0, 0xFF}
	gz = append(gz, make([]byte, 12)...)
	gz = append(gz, 0xE8, 0x03, 0, 0)
	writeOver(t, filepath.Join(home, "e2e", "d.0.e2e.test.gz"), gz)

	out, err := s.TryRun("dexpireover", "-R", "-v")
	var ee *exec.ExitError
	if errors.As(err, &ee) && ee.ExitCode() < 0 {
		t.Errorf("dexpireover -R died on the truncated gzip header (%v):\n%s", err, out)
	}
	if !strings.Contains(out, "bad gzip header") {
		t.Errorf("no report of the bad gzip header:\n%s", out)
	}
}

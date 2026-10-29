package e2e

import (
	"bufio"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// startReaderNoWait starts dreaderd -d for a reader site (written with NoStart) without
// waiting for a greeting, which a dreaderd without access definitions does not give.
func startReaderNoWait(tb testing.TB, s *Site) {
	tb.Helper()
	port := s.Addr[strings.LastIndex(s.Addr, ":")+1:]
	cmd := exec.Command(filepath.Join(s.Bin, "dreaderd"), "-d", "-p", PathHost, "-h", PathHost, "-P", "[127.0.0.1]:"+port)
	cmd.Dir = s.Dir
	cmd.Env = append(os.Environ(), s.Env...)
	if err := cmd.Start(); err != nil {
		tb.Fatal(err)
	}
	tb.Cleanup(func() { cmd.Process.Kill(); cmd.Wait() })
}

// readerGreeting dials addr and returns the first line of the answer ("" for none).
func readerGreeting(addr string) string {
	conn, err := net.DialTimeout("tcp", addr, time.Second)
	if err != nil {
		return ""
	}
	defer conn.Close()
	conn.SetReadDeadline(time.Now().Add(time.Second))
	line, _ := bufio.NewReader(conn).ReadString('\n')
	return line
}

func generalLog(s *Site) string {
	b, _ := os.ReadFile(filepath.Join(s.Dir, "log", "general.log"))
	return string(b)
}

var badReaderdefNumbers = []string{"maxconnperhost 1x", "idletimeout abc", "maxconn 99999999999", "bytelimit -5"}

func accessWith(line string) string {
	return strings.Replace(DefaultReaderAccess, "readerdef rp\n", "readerdef rp\n    "+line+"\n", 1)
}

// TestReaderdefNumbersValidated (D-161): a readerdef number that is not all digits (or is out
// of range) is logged ("bad value") and makes dreader.access invalid: it is not installed.
// On a first start there is no other access cache, so dreaderd gives no service; atoi() made
// "abc" 0 and "1x" 1, and dreaderd served with limits the admin never wrote.
func TestReaderdefNumbersValidated(t *testing.T) {
	for _, bad := range badReaderdefNumbers {
		t.Run(strings.ReplaceAll(bad, " ", "_"), func(t *testing.T) {
			s := NewReaderSite(t, ReaderOptions{SpoolAddr: "127.0.0.1:1", Access: accessWith(bad), NoStart: true,
				Config: map[string]string{"path_generallog": "%s/general.log"}})
			startReaderNoWait(t, s)
			var log string
			waitFor(t, 5*time.Second, "bad value in the log", func() bool {
				log = generalLog(s)
				return strings.Contains(log, "bad value")
			})
			if !strings.Contains(log, "'"+strings.Fields(bad)[0]+"'") {
				t.Errorf("the log does not name the option %q:\n%s", bad, log)
			}
			if g := readerGreeting(s.Addr); strings.HasPrefix(g, "200") || strings.HasPrefix(g, "201") {
				t.Errorf("dreaderd with %q in dreader.access gave service: %q", bad, strings.TrimSpace(g))
			}
		})
	}
}

// TestReaderdefNumbersRejectedReload (D-161): a dreader.access that gets a bad number while
// dreaderd runs is refused (logged) and the access cache from before keeps serving.
func TestReaderdefNumbersRejectedReload(t *testing.T) {
	s := NewReaderSite(t, ReaderOptions{SpoolAddr: "127.0.0.1:1",
		Config: map[string]string{"path_generallog": "%s/general.log"}})
	if g := readerGreeting(s.Addr); !strings.HasPrefix(g, "200") && !strings.HasPrefix(g, "201") {
		t.Fatalf("no service before the change: %q", g)
	}
	s.WriteFile(t, "dreader.access", accessWith("maxconnperhost 1x"))
	// a new connection makes dreaderd look at dreader.access
	var log string
	waitFor(t, 10*time.Second, "bad value in the log", func() bool {
		readerGreeting(s.Addr)
		log = generalLog(s)
		return strings.Contains(log, "bad value")
	})
	if g := readerGreeting(s.Addr); !strings.HasPrefix(g, "200") && !strings.HasPrefix(g, "201") {
		t.Errorf("after the rejected reload dreaderd answers %q, want the old access cache to serve", g)
	}
}

// TestReaderdefNumberComment (D-161): an inline "# comment" after a readerdef number is a
// comment, as it was with atoi().
func TestReaderdefNumberComment(t *testing.T) {
	s := NewReaderSite(t, ReaderOptions{SpoolAddr: "127.0.0.1:1", Access: accessWith("maxconn 50 # at most fifty"),
		Config: map[string]string{"path_generallog": "%s/general.log"}})
	if g := readerGreeting(s.Addr); !strings.HasPrefix(g, "200") && !strings.HasPrefix(g, "201") {
		t.Errorf("dreaderd with an inline comment after a number answers %q, want service", g)
	}
	if log := generalLog(s); strings.Contains(log, "bad value") {
		t.Errorf("the comment was read as part of the number:\n%s", log)
	}
}

// TestAccessCacheWithStdinClosed (D-160): with file descriptor 0 closed the first file dreaderd
// opens gets descriptor 0, which is a valid descriptor. The access cache loader took 0 for a
// failed open, did not load the caches and the reader gave no service.
func TestAccessCacheWithStdinClosed(t *testing.T) {
	s := NewReaderSite(t, ReaderOptions{SpoolAddr: "127.0.0.1:1", NoStart: true,
		Config: map[string]string{"path_generallog": "%s/general.log"}})
	port := s.Addr[strings.LastIndex(s.Addr, ":")+1:]
	cmd := exec.Command("/bin/sh", "-c", `exec "$0" "$@" <&-`, filepath.Join(s.Bin, "dreaderd"),
		"-d", "-p", PathHost, "-h", PathHost, "-P", "[127.0.0.1]:"+port)
	cmd.Dir = s.Dir
	cmd.Env = append(os.Environ(), s.Env...)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cmd.Process.Kill(); cmd.Wait() })
	var g string
	waitFor(t, 10*time.Second, "a greeting", func() bool {
		g = readerGreeting(s.Addr)
		return g != ""
	})
	if !strings.HasPrefix(g, "200") && !strings.HasPrefix(g, "201") {
		t.Errorf("dreaderd started with stdin closed answers %q, want the greeting", g)
	}
}

// TestDserverHostsNumbers (D-162): dserver.hosts numbers are read whole. A flag number that
// overflows and a cache option that is not a number are logged and ignored (the entry keeps
// its defaults) instead of becoming a clamped or a zero value; a good entry is not touched.
func TestDserverHostsNumbers(t *testing.T) {
	_, spool := spoolStore(t)
	host, port, _ := net.SplitHostPort(spool.Addr())
	hosts := "127.0.0.2\ts1p99999999999999\n" +
		"127.0.0.3\ts1p119 cachemin=abc cachemax=12junk\n" +
		host + "\ts1p" + port + "\n"
	s := NewReaderSite(t, ReaderOptions{Spool: spool, Files: map[string]string{"dserver.hosts": hosts},
		Config: map[string]string{"path_generallog": "%s/general.log"}})
	c := s.Reader(t)
	expect(t, c, 211, "GROUP e2e.test")
	expect(t, c, 205, "QUIT")
	var log string
	waitFor(t, 10*time.Second, "the log lines", func() bool {
		log = generalLog(s)
		return strings.Contains(log, "bad number after 'p'") && strings.Contains(log, "cachemin=abc") && strings.Contains(log, "cachemax=12junk")
	})
}

package e2e

// samples/adm/diablo-ctl: start, stop, restart and status for diablo and dreaderd.

import (
	"bytes"
	"context"
	"errors"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	testkit "github.com/go-while/nntp-testkit"
)

// EnvCtl names the environment variable with the path of diablo-ctl; default: the source
// tree's samples/adm/diablo-ctl next to DIABLO_BIN (a dbin directory of a build).
const EnvCtl = "DIABLO_CTL"

// ctlPath returns diablo-ctl's path or skips the test.
func ctlPath(tb testing.TB, s *Site) string {
	tb.Helper()
	p := os.Getenv(EnvCtl)
	if p == "" {
		p = filepath.Join(s.Bin, "..", "samples", "adm", "diablo-ctl")
	}
	if _, err := os.Stat(p); err != nil {
		tb.Skipf("no diablo-ctl at %s (set %s)", p, EnvCtl)
	}
	return p
}

// ctl runs diablo-ctl with the site's diablo.config and conf, and returns its output and
// exit status.
func ctl(tb testing.TB, s *Site, ctlPath, conf string, args ...string) (string, int) {
	tb.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*testkit.Deadline())
	defer cancel()
	a := append([]string{ctlPath, "-C", filepath.Join(s.Dir, "diablo.config"), "-f", conf}, args...)
	cmd := exec.CommandContext(ctx, "/bin/sh", a...)
	cmd.Dir = s.Dir
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	cmd.WaitDelay = 3 * time.Second // the started daemon keeps our output pipe
	err := cmd.Run()
	code := 0
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		code = ee.ExitCode()
	} else if err != nil && !errors.Is(err, exec.ErrWaitDelay) {
		tb.Fatalf("diablo-ctl %s: %v\n%s", strings.Join(args, " "), err, out.String())
	}
	return out.String(), code
}

func wantCtl(tb testing.TB, out string, code, want int, what string) {
	tb.Helper()
	if code != want {
		tb.Fatalf("diablo-ctl %s: exit status %d, want %d\n%s", what, code, want, out)
	}
}

// TestDiabloCtlFeeder: diablo-ctl status/start/stop/restart for diablo on a fresh site.
func TestDiabloCtlFeeder(t *testing.T) {
	s := NewSite(t, SiteOptions{NoStart: true})
	ctlp := ctlPath(t, s)
	conf := filepath.Join(s.Dir, "diablo-ctl.test.conf")
	s.WriteFile(t, "diablo-ctl.test.conf", "DIABLO_BIN='"+s.Bin+"'\n"+
		"DIABLO_DAEMONS='diablo'\n"+
		"DIABLO_OPTS='-p "+PathHost+" -h "+PathHost+" -P "+s.Addr+"'\n"+
		"DIABLO_STOP_WAIT=15\n")
	t.Cleanup(func() { ctl(t, s, ctlp, conf, "stop") })

	out, code := ctl(t, s, ctlp, conf, "status")
	wantCtl(t, out, code, 3, "status before start")

	out, code = ctl(t, s, ctlp, conf, "start")
	wantCtl(t, out, code, 0, "start")
	waitFor(t, testkit.Deadline(), "dhosts.cache", func() bool {
		_, err := os.Stat(filepath.Join(s.Dir, "dhosts.cache"))
		return err == nil
	})
	p := s.Peer(t)
	if !strings.HasPrefix(p.Greeting, "200 ") {
		t.Fatalf("peer greeting after diablo-ctl start: %q", p.Greeting)
	}
	expect(t, p, 205, "QUIT")

	out, code = ctl(t, s, ctlp, conf, "start")
	wantCtl(t, out, code, 0, "start while running")
	if !strings.Contains(out, "already running") {
		t.Errorf("second start: %q, want \"already running\"", out)
	}
	out, code = ctl(t, s, ctlp, conf, "status")
	wantCtl(t, out, code, 0, "status while running")

	out, code = ctl(t, s, ctlp, conf, "restart")
	wantCtl(t, out, code, 0, "restart")
	out, code = ctl(t, s, ctlp, conf, "status")
	wantCtl(t, out, code, 0, "status after restart")

	out, code = ctl(t, s, ctlp, conf, "stop")
	wantCtl(t, out, code, 0, "stop")
	if c, err := net.DialTimeout("tcp", s.Addr, time.Second); err == nil {
		c.Close()
		t.Errorf("diablo still listens on %s after diablo-ctl stop\n%s", s.Addr, out)
	}
	out, code = ctl(t, s, ctlp, conf, "status")
	wantCtl(t, out, code, 3, "status after stop")
	out, code = ctl(t, s, ctlp, conf, "stop")
	wantCtl(t, out, code, 0, "stop when not running")
}

// TestDiabloCtlReader: diablo-ctl status/start/stop for dreaderd.
func TestDiabloCtlReader(t *testing.T) {
	_, spool := spoolStore(t)
	s := NewReaderSite(t, ReaderOptions{Spool: spool, NoStart: true})
	ctlp := ctlPath(t, s)
	_, port, _ := net.SplitHostPort(s.Addr)
	conf := filepath.Join(s.Dir, "diablo-ctl.test.conf")
	s.WriteFile(t, "diablo-ctl.test.conf", "DIABLO_BIN='"+s.Bin+"'\n"+
		"DIABLO_DAEMONS='dreaderd'\n"+
		"DREADERD_OPTS='-p "+PathHost+" -h "+PathHost+" -P [127.0.0.1]:"+port+"'\n"+
		"DIABLO_STOP_WAIT=15\n")
	t.Cleanup(func() { ctl(t, s, ctlp, conf, "stop") })

	out, code := ctl(t, s, ctlp, conf, "status")
	wantCtl(t, out, code, 3, "status before start")
	out, code = ctl(t, s, ctlp, conf, "start")
	wantCtl(t, out, code, 0, "start")
	c := s.Reader(t)
	expect(t, c, 205, "QUIT")
	out, code = ctl(t, s, ctlp, conf, "status")
	wantCtl(t, out, code, 0, "status while running")
	if !strings.Contains(out, "pid ") || strings.Contains(out, "pid )") {
		t.Errorf("status without a pid: %q", out)
	}
	out, code = ctl(t, s, ctlp, conf, "stop")
	wantCtl(t, out, code, 0, "stop")
	if c, err := net.DialTimeout("tcp", s.Addr, time.Second); err == nil {
		c.Close()
		t.Errorf("dreaderd still listens on %s after diablo-ctl stop\n%s", s.Addr, out)
	}
	out, code = ctl(t, s, ctlp, conf, "status")
	wantCtl(t, out, code, 3, "status after stop")
}


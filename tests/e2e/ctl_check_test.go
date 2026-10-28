package e2e

// diablo-ctl check and the checks before a start, the dkp trim at start, the pid file after a
// stop; and the init wrappers in samples/init.

import (
	"encoding/xml"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	testkit "github.com/go-while/nntp-testkit"
)

// ctlSite writes a site (not started) and a diablo-ctl.conf with extra lines, and returns the
// site, diablo-ctl's path and a function that runs diablo-ctl.
func ctlSite(t *testing.T, config map[string]string, extra string) (*Site, func(args ...string) (string, int)) {
	t.Helper()
	s := NewSite(t, SiteOptions{NoStart: true, Config: config})
	ctlp := ctlPath(t, s)
	conf := filepath.Join(s.Dir, "diablo-ctl.test.conf")
	s.WriteFile(t, "diablo-ctl.test.conf", "DIABLO_BIN='"+s.Bin+"'\n"+
		"DIABLO_DAEMONS='diablo'\n"+
		"DIABLO_OPTS='-p "+PathHost+" -h "+PathHost+" -P "+s.Addr+"'\n"+
		"DIABLO_STOP_WAIT=15\n"+extra)
	t.Cleanup(func() { ctl(t, s, ctlp, conf, "stop") })
	return s, func(args ...string) (string, int) { return ctl(t, s, ctlp, conf, args...) }
}

// skipWithoutPreflight skips the test for a diablo-ctl that has no check action.
func skipWithoutPreflight(t *testing.T, run func(args ...string) (string, int)) {
	t.Helper()
	if out, code := run("check"); code == 2 || strings.Contains(out, "usage:") {
		t.Skip("diablo-ctl has no check action")
	}
}

func listening(addr string) bool {
	c, err := net.DialTimeout("tcp", addr, time.Second)
	if err != nil {
		return false
	}
	c.Close()
	return true
}

// TestDiabloCtlChecks: every check, with check and with start.
func TestDiabloCtlChecks(t *testing.T) {
	t.Run("nothing to complain about", func(t *testing.T) {
		_, run := ctlSite(t, nil, "")
		skipWithoutPreflight(t, run)
		out, code := run("check")
		wantCtl(t, out, code, 0, "check")
		if !strings.Contains(out, "checks passed") {
			t.Errorf("check: %q, want \"checks passed\"", out)
		}
	})

	t.Run("mounts", func(t *testing.T) {
		s, run := ctlSite(t, nil, "")
		skipWithoutPreflight(t, run)
		conf := filepath.Join(s.Dir, "diablo-ctl.test.conf")
		b, _ := os.ReadFile(conf)
		os.WriteFile(conf, append(b, []byte("DIABLO_MOUNTS='"+filepath.Join(s.Dir, "no-such-mount")+"'\n")...), 0o644)
		out, code := run("check")
		wantCtl(t, out, code, 1, "check with a missing mount")
		if !strings.Contains(out, "is not mounted") {
			t.Errorf("check: %q, want \"is not mounted\"", out)
		}
		out, code = run("start")
		wantCtl(t, out, code, 1, "start with a missing mount")
		if !strings.Contains(out, "a check failed") || listening(s.Addr) {
			t.Errorf("start: %q (listening %v), want a refusal", out, listening(s.Addr))
		}
		out, code = run("-F", "start")
		wantCtl(t, out, code, 0, "start -F")
		if !listening(s.Addr) {
			t.Errorf("diablo does not listen after start -F\n%s", out)
		}
	})

	t.Run("preflight off", func(t *testing.T) {
		s, run := ctlSite(t, nil, "DIABLO_PREFLIGHT=no\nDIABLO_MOUNTS='/no/such/mount'\n")
		skipWithoutPreflight(t, run)
		if out, code := run("start"); code != 0 || !listening(s.Addr) {
			t.Errorf("start with DIABLO_PREFLIGHT=no: exit %d\n%s", code, out)
		}
	})

	t.Run("history", func(t *testing.T) {
		// a missing history is diablo's to create: no failure, only a note when a snapshot exists
		s, run := ctlSite(t, nil, "")
		skipWithoutPreflight(t, run)
		os.Remove(filepath.Join(s.Dir, "dhistory"))
		out, code := run("check")
		wantCtl(t, out, code, 0, "check without a history file")
		if strings.Contains(out, "note:") {
			t.Errorf("check without a history file and without a snapshot: %q, want no note", out)
		}
		s.WriteFile(t, "dhistory.snp", "x")
		out, code = run("check")
		wantCtl(t, out, code, 0, "check with a snapshot only")
		if !strings.Contains(out, "note:") {
			t.Errorf("check with a snapshot only: %q, want a note", out)
		}
		out, code = run("start")
		wantCtl(t, out, code, 0, "start without a history file")
		if !listening(s.Addr) {
			t.Errorf("diablo makes its own history: not listening\n%s", out)
		}
	})

	t.Run("history required", func(t *testing.T) {
		s, run := ctlSite(t, nil, "DIABLO_REQUIRE_HISTORY=yes\n")
		skipWithoutPreflight(t, run)
		os.Remove(filepath.Join(s.Dir, "dhistory"))
		out, code := run("check")
		wantCtl(t, out, code, 1, "check with DIABLO_REQUIRE_HISTORY and no history")
		if !strings.Contains(out, "history file") {
			t.Errorf("check: %q", out)
		}
		s.WriteFile(t, "dhistory", "x")
		out, code = run("check")
		wantCtl(t, out, code, 0, "check with the history file")
	})

	t.Run("active file", func(t *testing.T) {
		s, run := ctlSite(t, map[string]string{"active": "on"}, "DIABLO_ACTIVE_MAXAGE=1\n")
		skipWithoutPreflight(t, run)
		os.Remove(filepath.Join(s.Dir, "dactive.kp"))
		out, code := run("check")
		wantCtl(t, out, code, 1, "check with active on and no active file")
		if !strings.Contains(out, "active file") || !strings.Contains(out, "not found") {
			t.Errorf("check: %q", out)
		}
		s.WriteFile(t, "dactive.kp", "")
		out, code = run("check")
		wantCtl(t, out, code, 0, "check with a fresh active file")
		old := time.Now().Add(-3 * time.Hour)
		if err := os.Chtimes(filepath.Join(s.Dir, "dactive.kp"), old, old); err != nil {
			t.Fatal(err)
		}
		out, code = run("check")
		wantCtl(t, out, code, 1, "check with a stale active file")
		if !strings.Contains(out, "hours old") {
			t.Errorf("check: %q, want \"hours old\"", out)
		}
	})

	t.Run("active off", func(t *testing.T) {
		s, run := ctlSite(t, nil, "")
		skipWithoutPreflight(t, run)
		os.Remove(filepath.Join(s.Dir, "dactive.kp"))
		out, code := run("check")
		wantCtl(t, out, code, 0, "check with active off and no active file")
	})

	t.Run("unclean shutdown", func(t *testing.T) {
		s, run := ctlSite(t, nil, "DIABLO_MAX_DOWNTIME=60\n")
		skipWithoutPreflight(t, run)
		if err := os.MkdirAll(filepath.Join(s.Dir, "log"), 0o755); err != nil {
			t.Fatal(err)
		}
		s.WriteFile(t, "log/diablo.pid", "999999\n")
		s.WriteFile(t, "log/article.log", "x\n")
		out, code := run("check")
		wantCtl(t, out, code, 0, "check with a pid file and a fresh log")
		old := time.Now().Add(-3 * time.Hour)
		if err := os.Chtimes(filepath.Join(s.Dir, "log", "article.log"), old, old); err != nil {
			t.Fatal(err)
		}
		out, code = run("check")
		wantCtl(t, out, code, 1, "check with a pid file and an old log")
		if !strings.Contains(out, "unclean shutdown") {
			t.Errorf("check: %q", out)
		}
		out, code = run("start")
		wantCtl(t, out, code, 1, "start after an unclean shutdown")
		if listening(s.Addr) {
			t.Error("diablo started after an unclean shutdown")
		}
		os.Remove(filepath.Join(s.Dir, "log", "diablo.pid"))
		out, code = run("check")
		wantCtl(t, out, code, 0, "check without the pid file")
	})
}

// TestDiabloCtlStopRemovesPid: dreaderd leaves its pid file when it exits; diablo-ctl stop
// takes it away, so that a pid file without a process means an unclean shutdown.
func TestDiabloCtlStopRemovesPid(t *testing.T) {
	_, spool := spoolStore(t)
	s := NewReaderSite(t, ReaderOptions{Spool: spool, NoStart: true})
	ctlp := ctlPath(t, s)
	conf := filepath.Join(s.Dir, "diablo-ctl.test.conf")
	_, port, _ := strings.Cut(s.Addr, ":")
	s.WriteFile(t, "diablo-ctl.test.conf", "DIABLO_BIN='"+s.Bin+"'\n"+
		"DIABLO_DAEMONS='dreaderd'\n"+
		"DREADERD_OPTS='-p "+PathHost+" -h "+PathHost+" -P [127.0.0.1]:"+port+"'\n"+
		"DIABLO_STOP_WAIT=15\n")
	t.Cleanup(func() { ctl(t, s, ctlp, conf, "stop") })
	pidFile := filepath.Join(s.Dir, "log", "dreaderd.pid")

	out, code := ctl(t, s, ctlp, conf, "start")
	wantCtl(t, out, code, 0, "start")
	if _, err := os.Stat(pidFile); err != nil {
		t.Fatalf("no pid file while dreaderd runs: %v", err)
	}
	out, code = ctl(t, s, ctlp, conf, "stop")
	wantCtl(t, out, code, 0, "stop")
	if _, err := os.Stat(pidFile); err == nil {
		t.Errorf("%s is still there after diablo-ctl stop", pidFile)
	}
}

// TestDiabloCtlDkpTrim: DIABLO_DKP_TRIM=yes trims the active file before a start, when no
// daemon of the site runs.
func TestDiabloCtlDkpTrim(t *testing.T) {
	s, run := ctlSite(t, nil, "DIABLO_DKP_TRIM=yes\n")
	skipWithoutPreflight(t, run)
	file := filepath.Join(s.Dir, "dactive.kp")
	s.Run(t, "dkp", "-w", file, "keep.this", "S==y")
	s.Run(t, "dkp", "-w", file, "drop.this", "S==y")
	s.Run(t, "dkp", "-w", "-x", file, "drop.this")
	if _, dead, _ := kpRecords(t, file, "drop.this"); len(dead) == 0 {
		t.Fatalf("no deleted record to trim in %s", file)
	}
	out, code := run("start")
	wantCtl(t, out, code, 0, "start with DIABLO_DKP_TRIM")
	if live, _, _ := kpRecords(t, file, "keep.this"); len(live) != 1 {
		t.Errorf("keep.this after the trim: %q, want one live record", live)
	}
	if live, dead, _ := kpRecords(t, file, "drop.this"); len(live) != 0 || len(dead) != 0 {
		t.Errorf("drop.this after the trim: live %q dead %q, want none", live, dead)
	}
	// with the daemon running the file is left alone
	s.Run(t, "dkp", "-w", file, "drop2.this", "S==y")
	s.Run(t, "dkp", "-w", "-x", file, "drop2.this")
	out, code = run("restart")
	wantCtl(t, out, code, 0, "restart")
	run("stop")
}

// TestInitWrappers: the files in samples/init are well formed and call diablo-ctl.
func TestInitWrappers(t *testing.T) {
	s := NewSite(t, SiteOptions{NoStart: true})
	root := filepath.Join(s.Bin, "..", "samples", "init")
	if _, err := os.Stat(root); err != nil {
		t.Skip("no samples/init")
	}
	for _, f := range []string{"freebsd/diablo", "freebsd/dreaderd", "sysv/diablo", "sysv/dreaderd"} {
		p := filepath.Join(root, f)
		if out, err := exec.Command("/bin/sh", "-n", p).CombinedOutput(); err != nil {
			t.Errorf("sh -n %s: %v\n%s", f, err, out)
		}
		b, _ := os.ReadFile(p)
		if !strings.Contains(string(b), "diablo-ctl") {
			t.Errorf("%s does not call diablo-ctl", f)
		}
		if fi, err := os.Stat(p); err != nil || fi.Mode()&0o111 == 0 {
			t.Errorf("%s is not executable", f)
		}
	}
	for _, f := range []string{"smf/diablo.xml", "smf/dreaderd.xml"} {
		b, err := os.ReadFile(filepath.Join(root, f))
		if err != nil {
			t.Fatal(err)
		}
		var v struct {
			Services []struct {
				Name    string `xml:"name,attr"`
				Methods []struct {
					Name string `xml:"name,attr"`
					Exec string `xml:"exec,attr"`
				} `xml:"exec_method"`
			} `xml:"service"`
		}
		if err := xml.Unmarshal(b, &v); err != nil || len(v.Services) != 1 {
			t.Fatalf("%s: %v, %d services", f, err, len(v.Services))
		}
		var start, stop string
		for _, m := range v.Services[0].Methods {
			switch m.Name {
			case "start":
				start = m.Exec
			case "stop":
				stop = m.Exec
			}
		}
		name := strings.TrimSuffix(filepath.Base(f), ".xml")
		if !strings.HasSuffix(start, "diablo-ctl start "+name) || !strings.HasSuffix(stop, "diablo-ctl stop "+name) {
			t.Errorf("%s: start %q stop %q", f, start, stop)
		}
	}
	for _, f := range []string{"systemd/diablo.service", "systemd/dreaderd.service"} {
		b, err := os.ReadFile(filepath.Join(root, f))
		if err != nil {
			t.Fatal(err)
		}
		name := strings.TrimSuffix(filepath.Base(f), ".service")
		for _, want := range []string{"ExecStart=/news/adm/diablo-ctl start " + name, "ExecStop=/news/adm/diablo-ctl stop " + name,
			"PIDFile=/news/log/" + name + ".pid", "Type=forking"} {
			if !strings.Contains(string(b), want) {
				t.Errorf("%s: no %q", f, want)
			}
		}
	}
}

// TestInitSysV: the SysV script of samples/init starts, checks and stops diablo through
// diablo-ctl, with DIABLO_CTL and DIABLO_CTL_OPTS from /etc/default/diablo replaced by the
// environment.
func TestInitSysV(t *testing.T) {
	s := NewSite(t, SiteOptions{NoStart: true})
	script := filepath.Join(s.Bin, "..", "samples", "init", "sysv", "diablo")
	if _, err := os.Stat(script); err != nil {
		t.Skip("no samples/init/sysv")
	}
	ctlp := ctlPath(t, s)
	conf := filepath.Join(s.Dir, "diablo-ctl.test.conf")
	s.WriteFile(t, "diablo-ctl.test.conf", "DIABLO_BIN='"+s.Bin+"'\nDIABLO_DAEMONS='diablo'\n"+
		"DIABLO_OPTS='-p "+PathHost+" -h "+PathHost+" -P "+s.Addr+"'\nDIABLO_STOP_WAIT=15\n")
	t.Cleanup(func() { ctl(t, s, ctlp, conf, "stop") })
	run := func(arg string) (string, int) {
		cmd := exec.Command("/bin/sh", script, arg)
		cmd.Dir = t.TempDir()
		cmd.Env = append(os.Environ(), "DIABLO_CTL="+ctlp, "DIABLO_CTL_OPTS=-C "+filepath.Join(s.Dir, "diablo.config")+" -f "+conf)
		out, err := cmd.CombinedOutput()
		code := 0
		if ee, ok := err.(*exec.ExitError); ok {
			code = ee.ExitCode()
		} else if err != nil {
			t.Fatal(err)
		}
		return string(out), code
	}
	// /etc/default/diablo would override DIABLO_CTL; a host that has one is not tested
	if _, err := os.Stat("/etc/default/diablo"); err == nil {
		t.Skip("/etc/default/diablo exists")
	}
	if out, code := run("status"); code != 3 {
		t.Fatalf("status before start: exit %d, want 3\n%s", code, out)
	}
	if out, code := run("start"); code != 0 || !listening(s.Addr) {
		t.Fatalf("start: exit %d, listening %v\n%s", code, listening(s.Addr), out)
	}
	if out, code := run("status"); code != 0 {
		t.Errorf("status: exit %d\n%s", code, out)
	}
	if out, code := run("check"); code != 0 {
		t.Errorf("check: exit %d\n%s", code, out)
	}
	if out, code := run("restart"); code != 0 || !listening(s.Addr) {
		t.Errorf("restart: exit %d, listening %v\n%s", code, listening(s.Addr), out)
	}
	if out, code := run("stop"); code != 0 || listening(s.Addr) {
		t.Errorf("stop: exit %d, listening %v\n%s", code, listening(s.Addr), out)
	}
	if out, code := run("bogus"); code != 2 {
		t.Errorf("an unknown action: exit %d, want 2\n%s", code, out)
	}
}

// TestInitRcdFreeBSD: the rc.d script runs with rc.subr (FreeBSD only) against its own site.
func TestInitRcdFreeBSD(t *testing.T) {
	if _, err := os.Stat("/etc/rc.subr"); err != nil {
		t.Skip("no /etc/rc.subr")
	}
	s := NewSite(t, SiteOptions{NoStart: true})
	script := filepath.Join(s.Bin, "..", "samples", "init", "freebsd", "diablo")
	if _, err := os.Stat(script); err != nil {
		t.Skip("no samples/init/freebsd")
	}
	ctlp := ctlPath(t, s)
	conf := filepath.Join(s.Dir, "diablo-ctl.test.conf")
	s.WriteFile(t, "diablo-ctl.test.conf", "DIABLO_BIN='"+s.Bin+"'\nDIABLO_DAEMONS='diablo'\n"+
		"DIABLO_OPTS='-p "+PathHost+" -h "+PathHost+" -P "+s.Addr+"'\nDIABLO_STOP_WAIT=15\n")
	t.Cleanup(func() { ctl(t, s, ctlp, conf, "stop") })
	run := func(arg string) (string, int) {
		cmd := exec.Command("/bin/sh", script, arg)
		cmd.Dir = t.TempDir()
		cmd.Env = append(os.Environ(), "diablo_ctl="+ctlp,
			"diablo_flags=-C "+filepath.Join(s.Dir, "diablo.config")+" -f "+conf)
		out, err := cmd.CombinedOutput()
		code := 0
		if ee, ok := err.(*exec.ExitError); ok {
			code = ee.ExitCode()
		} else if err != nil {
			t.Fatal(err)
		}
		return string(out), code
	}
	if out, code := run("onestatus"); code == 0 {
		t.Fatalf("onestatus before start: exit 0\n%s", out)
	}
	if out, code := run("onestart"); code != 0 || !listening(s.Addr) {
		t.Fatalf("onestart: exit %d, listening %v\n%s", code, listening(s.Addr), out)
	}
	if out, code := run("onestatus"); code != 0 {
		t.Errorf("onestatus: exit %d\n%s", code, out)
	}
	if out, code := run("check"); code != 0 {
		t.Errorf("check: exit %d\n%s", code, out)
	}
	if out, code := run("onestop"); code != 0 || listening(s.Addr) {
		t.Errorf("onestop: exit %d, listening %v\n%s", code, listening(s.Addr), out)
	}
}

// admScript runs samples/adm/name in the site (DIABLO_BIN, the site's config) and returns its
// output and exit status, or skips the test when the script is not there.
func admScript(t *testing.T, s *Site, name string, args ...string) (string, int) {
	t.Helper()
	p := filepath.Join(s.Bin, "..", "samples", "adm", name)
	if _, err := os.Stat(p); err != nil {
		t.Skipf("no samples/adm/%s", name)
	}
	cmd := exec.Command("/bin/sh", append([]string{p}, args...)...)
	cmd.Dir = t.TempDir()
	cmd.Env = append(append(os.Environ(), s.Env...), "DIABLO_BIN="+s.Bin)
	out, err := cmd.CombinedOutput()
	code := 0
	if ee, ok := err.(*exec.ExitError); ok {
		code = ee.ExitCode()
	} else if err != nil {
		t.Fatal(err)
	}
	return string(out), code
}

// TestAdmDumphist: dumphist keeps the newest copy of the history as dhistory.snp and the one
// before as dhistory.old.
func TestAdmDumphist(t *testing.T) {
	s := NewSite(t, SiteOptions{NoStart: true})
	hist := filepath.Join(s.Dir, "dhistory")
	s.WriteFile(t, "dhistory", "first")
	if out, code := admScript(t, s, "dumphist"); code != 0 {
		t.Fatalf("dumphist: exit %d\n%s", code, out)
	}
	if b, _ := os.ReadFile(hist + ".snp"); string(b) != "first" {
		t.Fatalf("dhistory.snp: %q, want the history", b)
	}
	s.WriteFile(t, "dhistory", "second")
	if out, code := admScript(t, s, "dumphist"); code != 0 {
		t.Fatalf("second dumphist: exit %d\n%s", code, out)
	}
	snp, _ := os.ReadFile(hist + ".snp")
	old, _ := os.ReadFile(hist + ".old")
	if string(snp) != "second" || string(old) != "first" {
		t.Errorf("dhistory.snp %q, dhistory.old %q; want second and first", snp, old)
	}
	if left, _ := filepath.Glob(hist + ".tmp.*"); len(left) != 0 {
		t.Errorf("temporary files left: %v", left)
	}
	// .old is the snapshot before, not a link to the newest one: a third run moves both on
	s.WriteFile(t, "dhistory", "third")
	if out, code := admScript(t, s, "dumphist"); code != 0 {
		t.Fatalf("third dumphist: exit %d\n%s", code, out)
	}
	snp, _ = os.ReadFile(hist + ".snp")
	old, _ = os.ReadFile(hist + ".old")
	if string(snp) != "third" || string(old) != "second" {
		t.Errorf("dhistory.snp %q, dhistory.old %q; want third and second", snp, old)
	}
	os.Remove(hist)
	if out, code := admScript(t, s, "dumphist"); code == 0 {
		t.Errorf("dumphist without a history file: exit 0\n%s", out)
	}
}

// TestAdmHistbackup: histbackup asks a running diablo for the snapshot (path_dumphist), does
// nothing without one, and says so when path_dumphist is not set.
func TestAdmHistbackup(t *testing.T) {
	s := NewSite(t, SiteOptions{NoStart: true})
	out, code := admScript(t, s, "histbackup")
	if code != 0 || !strings.Contains(out, "diablo is not running") {
		t.Errorf("histbackup without diablo: exit %d\n%s", code, out)
	}
	dump := filepath.Join(s.Bin, "..", "samples", "adm", "dumphist")
	s2 := NewSite(t, SiteOptions{Config: map[string]string{"path_dumphist": dump}})
	s2.WriteFile(t, "dhistory", "a history") // diablo makes its own file with the first article
	if out, code := admScript(t, s2, "histbackup"); code != 0 {
		t.Fatalf("histbackup with path_dumphist: exit %d\n%s", code, out)
	}
	waitFor(t, testkit.Deadline(), "dhistory.snp", func() bool {
		_, err := os.Stat(filepath.Join(s2.Dir, "dhistory.snp"))
		return err == nil
	})
	s3 := NewSite(t, SiteOptions{})
	out, code = admScript(t, s3, "histbackup")
	if code != 1 || !strings.Contains(out, "path_dumphist") {
		t.Errorf("histbackup without path_dumphist: exit %d, want 1 and a hint\n%s", code, out)
	}
}

// TestAdmActsync: usage, and nothing to do on a host that is one of the active file servers.
func TestAdmActsync(t *testing.T) {
	s := NewSite(t, SiteOptions{NoStart: true})
	if out, code := admScript(t, s, "actsync"); code != 2 || !strings.Contains(out, "usage:") {
		t.Errorf("actsync without a server: exit %d\n%s", code, out)
	}
	host, err := os.Hostname()
	if err != nil {
		t.Skip(err)
	}
	if out, code := admScript(t, s, "actsync", ","); code != 2 || !strings.Contains(out, "usage:") {
		t.Errorf("actsync with an empty server list: exit %d\n%s", code, out)
	}
	out, code := admScript(t, s, "actsync", "master.example.invalid,"+host)
	if code != 0 || !strings.Contains(out, "nothing to do") {
		t.Errorf("actsync on one of the servers: exit %d\n%s", code, out)
	}
}

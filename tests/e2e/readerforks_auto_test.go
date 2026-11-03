package e2e

// readerforks auto: dreaderd sizes its reader forks from the CPUs it may use (the affinity
// mask and a cgroup v2 quota on Linux, the cpuset on FreeBSD, the CPUs online elsewhere),
// capped at 64. A number still works as before, and a value that is neither is refused.

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/go-while/nntp-testkit/proc"
)

// readerForksAutoCap is READERFORKS_AUTO_CAP in dreaderd/main.c.
const readerForksAutoCap = 64

// readerForkPids returns the reader forks dreaderd -d reported on stdout ("forked reader
// child process N"). That line is printed on every system; syslog reaches stderr only where
// LOG_PERROR exists (not on illumos).
func readerForkPids(s *Site) map[string]bool {
	pids := map[string]bool{}
	re := regexp.MustCompile(`forked reader child process (\d+)`)
	for _, m := range re.FindAllStringSubmatch(s.Proc.Stderr(), -1) {
		pids[m[1]] = true
	}
	return pids
}

// expectReaderForks waits for want reader forks and checks that no more follow.
func expectReaderForks(t *testing.T, s *Site, want int) {
	t.Helper()
	waitFor(t, 20*time.Second, strconv.Itoa(want)+" reader forks", func() bool {
		return len(readerForkPids(s)) >= want
	})
	time.Sleep(time.Second)
	if n := len(readerForkPids(s)); n != want {
		t.Errorf("%d reader forks connected to the spool, want %d; output:\n%s",
			n, want, s.Proc.Stderr())
	}
}

// expectMaxConnects checks the connection limit in drcmd status (readerforks times
// readerthreads).
func expectMaxConnects(t *testing.T, s *Site, want int) {
	t.Helper()
	out := s.Run(t, "drcmd", "status")
	if !strings.Contains(out, "Act=0/"+strconv.Itoa(want)) {
		t.Errorf("drcmd status = %q, want Act=0/%d", strings.TrimSpace(out), want)
	}
}

// autoForks returns N from the "readerforks auto: N" log line, -1 when it is missing, or
// -2 when dreaderd's syslog does not reach stderr.
func autoForks(t *testing.T, s *Site) int {
	t.Helper()
	if !strings.Contains(s.Proc.Stderr(), "Waiting for connections") {
		return -2 // syslog is not on stderr here (no LOG_PERROR)
	}
	m := regexp.MustCompile(`readerforks auto: (\d+) \((\d+) CPUs available\)`).
		FindStringSubmatch(s.Proc.Stderr())
	if m == nil {
		return -1
	}
	n, _ := strconv.Atoi(m[1])
	return n
}

// cgroupCPULimit is the smallest cgroup v2 cpu.max on the path from our cgroup to the root,
// in whole CPUs rounded up; 0 when none is set or this is not Linux.
func cgroupCPULimit() int {
	b, err := os.ReadFile("/proc/self/cgroup")
	if err != nil {
		return 0
	}
	path := ""
	for _, l := range strings.Split(string(b), "\n") {
		if strings.HasPrefix(l, "0::/") {
			path = "/sys/fs/cgroup" + strings.TrimPrefix(l, "0::")
			break
		}
	}
	best := 0
	for path != "" && strings.HasPrefix(path, "/sys/fs/cgroup") {
		if b, err := os.ReadFile(filepath.Join(path, "cpu.max")); err == nil {
			f := strings.Fields(string(b))
			if len(f) == 2 {
				q, e1 := strconv.ParseInt(f[0], 10, 64)
				p, e2 := strconv.ParseInt(f[1], 10, 64)
				if e1 == nil && e2 == nil && q > 0 && p > 0 {
					if c := int((q + p - 1) / p); best == 0 || c < best {
						best = c
					}
				}
			}
		}
		if path == "/sys/fs/cgroup" {
			break
		}
		path = filepath.Dir(path)
	}
	return best
}

// expectedAuto is what readerforks auto should pick for a process allowed cpus CPUs.
func expectedAuto(cpus int) int {
	if q := cgroupCPULimit(); q > 0 && q < cpus {
		cpus = q
	}
	return max(1, min(cpus, readerForksAutoCap))
}

// TestReaderForksAuto: with readerforks auto and no affinity set, dreaderd starts one
// reader fork per CPU it may use (Go's runtime.NumCPU counts the same mask), logs the
// number, and its connection limit is that many forks times readerthreads.
// Fails on main: "auto" was read as 0, so no reader fork started.
func TestReaderForksAuto(t *testing.T) {
	want := expectedAuto(runtime.NumCPU())
	_, spool := spoolStore(t)
	s := NewReaderSite(t, ReaderOptions{Spool: spool,
		Config: map[string]string{"readerforks": "auto", "readerfeedforks": "0"}})
	if n := autoForks(t, s); n != -2 && n != want {
		t.Errorf("readerforks auto log line gives %d, want %d; output:\n%s", n, want, s.Proc.Stderr())
	}
	expectReaderForks(t, s, want)
	expectMaxConnects(t, s, want*4)
}

// TestReaderForksAutoTaskset: under taskset with two CPUs, readerforks auto starts two
// reader forks (one, if a cgroup quota allows only one CPU). Linux only.
func TestReaderForksAutoTaskset(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("taskset is Linux only")
	}
	taskset, err := exec.LookPath("taskset")
	if err != nil {
		t.Skip("taskset not installed")
	}
	cpus := allowedCPUs(t)
	if len(cpus) < 2 {
		t.Skipf("this process may use %d CPU(s), the test needs 2", len(cpus))
	}
	want := expectedAuto(2)

	_, spool := spoolStore(t)
	s := NewReaderSite(t, ReaderOptions{Spool: spool, NoStart: true,
		Config: map[string]string{"readerforks": "auto", "readerfeedforks": "0"}})
	port := strings.TrimPrefix(s.Addr, "127.0.0.1:")
	s.Proc = proc.StartTCP(t, proc.Options{
		Path: taskset,
		Args: []string{"-c", cpus[0] + "," + cpus[1], filepath.Join(s.Bin, "dreaderd"),
			"-d", "-p", PathHost, "-h", PathHost, "-P", "[127.0.0.1]:" + port},
		Env: s.Env,
		Dir: s.Dir,
	}, s.Addr)
	if n := autoForks(t, s); n != -2 && n != want {
		t.Errorf("readerforks auto log line gives %d, want %d; output:\n%s", n, want, s.Proc.Stderr())
	}
	expectReaderForks(t, s, want)
	expectMaxConnects(t, s, want*4)
}

// TestReaderForksAutoCgroupQuota: in a cgroup v2 scope with a quota of 1.5 CPUs
// (systemd-run --user -p CPUQuota=150%), readerforks auto rounds up to two reader forks.
// Skipped where the user systemd cannot set a CPU quota.
func TestReaderForksAutoCgroupQuota(t *testing.T) {
	if runtime.GOOS != "linux" || runtime.NumCPU() < 2 {
		t.Skip("needs Linux and at least 2 CPUs")
	}
	sdrun, err := exec.LookPath("systemd-run")
	if err != nil {
		t.Skip("systemd-run not installed")
	}
	scope := []string{"--user", "--scope", "-q", "-p", "CPUQuota=150%"}
	probe := exec.Command(sdrun, append(scope, "sh", "-c",
		`cat "/sys/fs/cgroup$(sed -n 's/^0:://p' /proc/self/cgroup)/cpu.max"`)...)
	if out, err := probe.CombinedOutput(); err != nil || !strings.HasPrefix(string(out), "150000 100000") {
		t.Skipf("no CPU quota through systemd-run --user: %v %q", err, out)
	}

	_, spool := spoolStore(t)
	s := NewReaderSite(t, ReaderOptions{Spool: spool, NoStart: true,
		Config: map[string]string{"readerforks": "auto", "readerfeedforks": "0"}})
	port := strings.TrimPrefix(s.Addr, "127.0.0.1:")
	s.Proc = proc.StartTCP(t, proc.Options{
		Path: sdrun,
		Args: append(scope, filepath.Join(s.Bin, "dreaderd"),
			"-d", "-p", PathHost, "-h", PathHost, "-P", "[127.0.0.1]:"+port),
		Env: s.Env,
		Dir: s.Dir,
	}, s.Addr)
	if n := autoForks(t, s); n != -2 && n != 2 {
		t.Errorf("readerforks auto log line gives %d, want 2; output:\n%s", n, s.Proc.Stderr())
	}
	expectReaderForks(t, s, 2)
}

// allowedCPUs returns the CPU numbers this process may run on (Cpus_allowed_list).
func allowedCPUs(t *testing.T) []string {
	t.Helper()
	b, err := os.ReadFile("/proc/self/status")
	if err != nil {
		t.Skipf("no /proc/self/status: %v", err)
	}
	var cpus []string
	for _, l := range strings.Split(string(b), "\n") {
		v, ok := strings.CutPrefix(l, "Cpus_allowed_list:")
		if !ok {
			continue
		}
		for _, r := range strings.Split(strings.TrimSpace(v), ",") {
			lo, hi, isRange := strings.Cut(r, "-")
			a, err1 := strconv.Atoi(lo)
			z := a
			var err2 error
			if isRange {
				z, err2 = strconv.Atoi(hi)
			}
			if err1 != nil || err2 != nil {
				t.Fatalf("Cpus_allowed_list %q", v)
			}
			for c := a; c <= z && len(cpus) < 2; c++ {
				cpus = append(cpus, strconv.Itoa(c))
			}
		}
	}
	return cpus
}

// TestReaderForksNumber: a number is used as it is, with no auto sizing.
func TestReaderForksNumber(t *testing.T) {
	_, spool := spoolStore(t)
	s := NewReaderSite(t, ReaderOptions{Spool: spool,
		Config: map[string]string{"readerforks": "3", "readerfeedforks": "0"}})
	expectReaderForks(t, s, 3)
	expectMaxConnects(t, s, 12)
	if strings.Contains(s.Proc.Stderr(), "readerforks auto") {
		t.Errorf("readerforks 3 logged auto sizing; output:\n%s", s.Proc.Stderr())
	}
}

// TestReaderForksInvalid: a readerforks value that is neither auto nor 0..1024, in
// diablo.config or as -M, stops dreaderd at startup with an error, as other invalid
// diablo.config values do. Fails on main: "bogus" and "3x" were read as 0 and 3, and
// dreaderd started.
func TestReaderForksInvalid(t *testing.T) {
	_, spool := spoolStore(t)
	for _, c := range []struct{ conf, flag string }{
		{conf: "bogus"},
		{conf: "3x"},
		{conf: "-1"},
		{conf: "1025"},
		{conf: "1", flag: "bogus"},
		{conf: "1", flag: "-1"},
	} {
		name := "config " + c.conf
		if c.flag != "" {
			name = "-M " + c.flag
		}
		t.Run(name, func(t *testing.T) {
			s := NewReaderSite(t, ReaderOptions{Spool: spool, NoStart: true})
			conf := s.ReadFile(t, "diablo.config")
			s.WriteFile(t, "diablo.config",
				strings.Replace(conf, "readerforks\t1\n", "readerforks\t"+c.conf+"\n", 1))
			args := []string{"-d", "-p", PathHost, "-h", PathHost,
				"-P", "[127.0.0.1]:" + strconv.Itoa(freePort(t))}
			if c.flag != "" {
				args = append(args, "-M", c.flag)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, filepath.Join(s.Bin, "dreaderd"), args...)
			cmd.Dir = s.Dir
			cmd.Env = append(os.Environ(), s.Env...)
			out, err := cmd.CombinedOutput()
			if ctx.Err() != nil {
				t.Fatalf("dreaderd started with readerforks %q; output:\n%s", name, out)
			}
			if err == nil {
				t.Fatalf("dreaderd exited 0 with readerforks %q; output:\n%s", name, out)
			}
			if !strings.Contains(string(out), "readerforks must be auto or 0..1024") {
				t.Errorf("no readerforks error for %q; output:\n%s", name, out)
			}
		})
	}
}

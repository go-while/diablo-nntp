package e2e

// readerforks auto: dreaderd sizes its reader forks from the CPUs it may use (the affinity
// mask and a cgroup v2 quota on Linux, the cpuset on FreeBSD, the CPUs online lowered to the
// processor set and zone.cpu-cap on illumos and Solaris, the CPUs online elsewhere), capped
// at 64. A number still works as before, and a value that is neither is refused.

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
func expectedAuto(t *testing.T, cpus int) int {
	if q := cgroupCPULimit(); q > 0 && q < cpus {
		cpus = q
	}
	if isSun() {
		if n, _ := psetCPUs(t); n > 0 && n < cpus {
			cpus = n
		}
		if c, _ := zoneCPUCap(t); c > 0 && (c+99)/100 < cpus {
			cpus = (c + 99) / 100
		}
	}
	return max(1, min(cpus, readerForksAutoCap))
}

func isSun() bool { return runtime.GOOS == "illumos" || runtime.GOOS == "solaris" }

// sunTool finds an illumos/Solaris tool in PATH or its usual directories; "" when missing.
func sunTool(name string) string {
	if p, err := exec.LookPath(name); err == nil {
		return p
	}
	for _, d := range []string{"/usr/bin", "/usr/sbin"} {
		if p := filepath.Join(d, name); fileExists(p) {
			return p
		}
	}
	return ""
}

// zoneCPUCap returns zone.cpu-cap for this process (percent of one CPU: 800 is 8 CPUs) as
// prctl -P shows it, and that output. As dreaderd and Go's runtime do, it is the smallest
// basic or privileged value with a deny action that is not the maximal one ("max" or "inf"
// in the flag column); 0 when there is none or prctl is missing.
func zoneCPUCap(t *testing.T) (int, string) {
	prctl := sunTool("prctl")
	if prctl == "" {
		return 0, ""
	}
	out, err := exec.Command(prctl, "-P", "-n", "zone.cpu-cap", strconv.Itoa(os.Getpid())).CombinedOutput()
	if err != nil {
		t.Logf("prctl -P -n zone.cpu-cap: %v %q", err, out)
		return 0, string(out)
	}
	best := 0
	// "zone.cpu-cap privileged 800 - deny -": name, privilege, value, flag, action, recipient
	for _, l := range strings.Split(string(out), "\n") {
		f := strings.Fields(l)
		if len(f) < 5 || f[0] != "zone.cpu-cap" || (f[1] != "basic" && f[1] != "privileged") ||
			f[3] == "max" || f[3] == "inf" || !strings.HasPrefix(f[4], "deny") {
			continue
		}
		if v, err := strconv.Atoi(f[2]); err == nil && v > 0 && (best == 0 || v < best) {
			best = v
		}
	}
	return best, string(out)
}

// psetCPUs returns the CPUs in the processor set this process is bound to (psrset -q, then
// psrset -i), and what psrset printed; 0 when it is bound to none or psrset is missing.
func psetCPUs(t *testing.T) (int, string) {
	psrset := sunTool("psrset")
	if psrset == "" {
		return 0, ""
	}
	q, err := exec.Command(psrset, "-q", strconv.Itoa(os.Getpid())).CombinedOutput()
	// "process id 1234: 1", or "process id 1234: not bound"
	m := regexp.MustCompile(`process id \d+: (\d+)`).FindSubmatch(q)
	if err != nil || m == nil {
		return 0, string(q)
	}
	i, err := exec.Command(psrset, "-i", string(m[1])).CombinedOutput()
	out := string(q) + string(i)
	if err != nil {
		t.Logf("psrset -i %s: %v %q", m[1], err, i)
		return 0, out
	}
	// "user processor set 1: processors 2 3" (or "processor 2"): count the numbers after
	// the colon
	list := string(i)
	if k := strings.LastIndex(list, ":"); k >= 0 {
		list = list[k+1:]
	}
	n := 0
	for _, f := range strings.Fields(list) {
		if _, err := strconv.Atoi(f); err == nil {
			n++
		}
	}
	return n, out
}

// psrinfoOnline counts the CPUs psrinfo shows on-line or no-intr (what
// sysconf(_SC_NPROCESSORS_ONLN) counts; in a zone with its own pool, only the pool's CPUs),
// and returns that output; 0 when psrinfo is missing.
func psrinfoOnline(t *testing.T) (int, string) {
	psrinfo := sunTool("psrinfo")
	if psrinfo == "" {
		return 0, ""
	}
	out, err := exec.Command(psrinfo).CombinedOutput()
	if err != nil {
		t.Logf("psrinfo: %v %q", err, out)
		return 0, string(out)
	}
	n := 0
	for _, l := range strings.Split(string(out), "\n") {
		if f := strings.Fields(l); len(f) >= 2 && (f[1] == "on-line" || f[1] == "no-intr") {
			n++
		}
	}
	return n, string(out)
}

// TestReaderForksAuto: with readerforks auto and no affinity set, dreaderd starts one
// reader fork per CPU it may use (Go's runtime.NumCPU counts the same mask, and on illumos
// the same zone.cpu-cap), logs the number, and its connection limit is that many forks
// times readerthreads.
// Fails on main: "auto" was read as 0, so no reader fork started.
func TestReaderForksAuto(t *testing.T) {
	want := expectedAuto(t, runtime.NumCPU())
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
	want := expectedAuto(t, 2)

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

// TestReaderForksAutoZone: on illumos and Solaris, readerforks auto starts one reader fork
// per CPU online (psrinfo), lowered to the processor set the process is bound to (psrset)
// and to the zone's CPU cap (prctl zone.cpu-cap, rounded up). It reads these with the
// system tools rather than trusting runtime.NumCPU. Where no cap or set applies (as on the
// OmniOS CI VM), it checks the CPUs online. Fails on main in a capped zone: a zone with
// 56 CPUs and zone.cpu-cap 800 started 56 reader forks, not 8.
func TestReaderForksAutoZone(t *testing.T) {
	if !isSun() {
		t.Skip("illumos and Solaris only")
	}
	if sunTool("psrinfo") == "" || sunTool("prctl") == "" {
		t.Skip("psrinfo or prctl not installed")
	}
	online, info := psrinfoOnline(t)
	if online == 0 {
		t.Skipf("psrinfo shows no CPU on-line:\n%s", info)
	}
	pset, psout := psetCPUs(t)
	capPct, prout := zoneCPUCap(t)
	t.Logf("psrinfo: %d on-line; psrset: %d CPUs; zone.cpu-cap: %d; runtime.NumCPU: %d\n%s%s%s",
		online, pset, capPct, runtime.NumCPU(), info, psout, prout)

	cpus := online
	if pset > 0 && pset < cpus {
		cpus = pset
	}
	if c := (capPct + 99) / 100; capPct > 0 && c < cpus {
		cpus = c
	}
	want := max(1, min(cpus, readerForksAutoCap))
	if goWant := expectedAuto(t, runtime.NumCPU()); goWant != want {
		t.Logf("runtime.NumCPU gives %d reader forks, the system tools %d", goWant, want)
	}

	_, spool := spoolStore(t)
	s := NewReaderSite(t, ReaderOptions{Spool: spool,
		Config: map[string]string{"readerforks": "auto", "readerfeedforks": "0"}})
	if n := autoForks(t, s); n != -2 && n != want {
		t.Errorf("readerforks auto log line gives %d, want %d; output:\n%s", n, want, s.Proc.Stderr())
	}
	expectReaderForks(t, s, want)
	expectMaxConnects(t, s, want*4)
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

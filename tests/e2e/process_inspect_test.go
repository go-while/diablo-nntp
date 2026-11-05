package e2e

// The Linux callers retain their /proc paths. BSD ps and illumos pargs cover
// both children of the site master and workers orphaned after dspoolout exits.

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"
)

func inspectionOutput(name string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = append(os.Environ(), "LC_ALL=C")
	return cmd.Output()
}

func requireProcessInspection(t *testing.T, descriptors bool) {
	t.Helper()
	if runtime.GOOS == "linux" {
		return
	}
	var tools []string
	switch runtime.GOOS {
	case "freebsd":
		tools = []string{"ps"}
		if descriptors {
			tools = append(tools, "procstat")
		}
	case "netbsd":
		tools = []string{"ps"}
		if descriptors {
			tools = append(tools, "fstat")
		}
	case "solaris":
		tools = []string{"ps", "pargs"}
		if descriptors {
			tools = append(tools, "pfiles")
		}
	default:
		t.Skip("process inspection needs Linux, FreeBSD, NetBSD or illumos")
	}
	for _, tool := range tools {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("process inspection needs %s: %v", tool, err)
		}
	}
	if ps, err := inspectProcesses(); err != nil || !slices.ContainsFunc(ps, func(p inspectedProcess) bool { return p.pid == os.Getpid() }) {
		t.Fatalf("cannot inspect process table: %v", err)
	}
	if dir := os.Getenv(EnvBin); dir != "" {
		b, err := processEnvironment(os.Getpid())
		if err != nil || !hasSiteEnvironment(string(b), EnvBin+"="+dir) {
			t.Fatalf("cannot inspect process environment (needed for detached workers): %v", err)
		}
	}
	if descriptors {
		if _, err := inspectDescriptors(os.Getpid()); err != nil {
			t.Fatalf("cannot inspect own descriptors: %v", err)
		}
	}
}

type inspectedProcess struct {
	pid, parent int
	name        string
}

func inspectProcesses() ([]inspectedProcess, error) {
	args := []string{"-ax", "-o", "pid=", "-o", "ppid=", "-o", "ucomm="}
	if runtime.GOOS == "solaris" {
		// /usr/bin/ps on illumos uses System V selection, not BSD -ax.
		args = []string{"-e", "-o", "pid=", "-o", "ppid=", "-o", "fname="}
	}
	b, err := inspectionOutput("ps", args...)
	if err != nil {
		return nil, err
	}
	ps := parseProcesses(string(b))
	if len(ps) == 0 {
		return nil, fmt.Errorf("no process records in ps output %q", b)
	}
	return ps, nil
}

func parseProcesses(out string) []inspectedProcess {
	var ps []inspectedProcess
	for _, line := range strings.Split(out, "\n") {
		f := strings.Fields(line)
		if len(f) != 3 {
			continue
		}
		pid, e1 := strconv.Atoi(f[0])
		parent, e2 := strconv.Atoi(f[1])
		if e1 == nil && e2 == nil && pid > 0 && parent >= 0 {
			ps = append(ps, inspectedProcess{pid, parent, filepath.Base(f[2])})
		}
	}
	return ps
}

func processEnvironment(pid int) ([]byte, error) {
	if runtime.GOOS == "solaris" {
		return inspectionOutput("pargs", "-e", strconv.Itoa(pid))
	}
	return inspectionOutput("ps", "eww", "-p", strconv.Itoa(pid), "-o", "command=")
}

func hasSiteEnvironment(out, want string) bool {
	// ps prints environment entries as words; pargs prints "envp[N]: value".
	// Test-site paths contain no whitespace. Match a complete entry, not a
	// prefix which could select a neighbouring site's workers for SIGKILL.
	return slices.Contains(strings.Fields(out), want)
}

func portableSitePids(s *Site, name string) ([]int, error) {
	ps, err := inspectProcesses()
	if err != nil {
		return nil, err
	}
	owned := map[int]bool{}
	if s.Proc != nil {
		select {
		case <-s.Proc.Done():
			// A reaped master's PID may already belong to another process.
		default:
			owned = processDescendants(ps, s.Proc.Pid())
		}
	}
	var pids []int
	for _, p := range ps {
		if p.name != name {
			continue
		}
		if !owned[p.pid] {
			// dspoolout and its replacement dnewslinks are not descendants
			// of the feeder master. Recheck their exact inherited site tag.
			b, err := processEnvironment(p.pid)
			if err != nil || !hasSiteEnvironment(string(b), s.Env[0]) {
				continue
			}
		}
		pids = append(pids, p.pid)
	}
	slices.Sort(pids)
	return pids, nil
}

func processDescendants(ps []inspectedProcess, root int) map[int]bool {
	owned := map[int]bool{root: true}
	for changed := true; changed; {
		changed = false
		for _, p := range ps {
			if owned[p.parent] && !owned[p.pid] {
				owned[p.pid], changed = true, true
			}
		}
	}
	return owned
}

func portableRealtimeWorker(t testing.TB, s *Site) int {
	for _, pid := range sitePids(t, s, "dnewslink") {
		var b []byte
		var err error
		if runtime.GOOS == "solaris" {
			b, err = inspectionOutput("pargs", "-a", strconv.Itoa(pid))
		} else {
			b, err = inspectionOutput("ps", "-ww", "-p", strconv.Itoa(pid), "-o", "args=")
		}
		if err == nil && slices.ContainsFunc(strings.Fields(string(b)), func(arg string) bool { return strings.HasPrefix(arg, "-r") }) {
			return pid
		}
		// BSD setproctitle() replaces the argument display, including -r.
		// A realtime worker keeps the active (unsequenced) queue open;
		// batch workers hold fake.SNNNNN instead. Do not infer the role
		// from the mutable status title or accidentally count a batch PID.
		if holdsRealtimeQueue(s, pid) {
			return pid
		}
	}
	return 0
}

func holdsRealtimeQueue(s *Site, pid int) bool {
	queue := filepath.Join(s.Dir, "dqueue", OutLabel)
	if _, err := os.Stat(queue); err != nil {
		return false // between rotation and creation of the new active queue
	}
	if runtime.GOOS == "netbsd" {
		b, err := inspectionOutput("fstat", "-p", strconv.Itoa(pid), queue)
		return err == nil && len(parseDescriptors("netbsd", pid, string(b))) > 0
	}
	fds, err := inspectDescriptors(pid)
	if err != nil {
		return false // a worker may retire during the snapshot
	}
	resolved, _ := filepath.EvalSymlinks(queue)
	return slices.ContainsFunc(fds, func(fd inspectedFD) bool {
		return descriptorNamesFile(fd, queue, resolved)
	})
}

func descriptorNamesFile(fd inspectedFD, names ...string) bool {
	for _, name := range names {
		if name != "" && slices.Contains(strings.Fields(fd.target), name) {
			return true
		}
	}
	return false
}

func portableProcessStatus(pid int) string {
	state := "stat="
	if runtime.GOOS == "solaris" {
		state = "s="
	}
	b, err := inspectionOutput("ps", "-p", strconv.Itoa(pid), "-o", "pid=", "-o", "ppid=", "-o", state, "-o", "wchan=")
	return fmt.Sprintf("%s (ps: %v)", strings.TrimSpace(string(b)), err)
}

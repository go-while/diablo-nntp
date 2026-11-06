package e2e

// Opt-in startup observation. Capture BEFORE proc.StartTCP's cleanup kills the
// server, not in a later test cleanup after the evidence died.

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/go-while/nntp-testkit/proc"
)

const startupDiagnosticsEnv = "E2E_STARTUP_DIAGNOSTICS"
const startupDiagnosticsDirEnv = "E2E_STARTUP_DIAGNOSTICS_DIR"

type startupObservation struct {
	Test, Executable, Site, Address, Token string
	Started, Ready                         time.Time
	PID                                    int
	Failure                                string `json:",omitempty"`
}

// With diagnostics disabled this calls exactly the original readiness helper.
// When enabled, a tiny exec launcher records the real server PID and an inherited
// per-launch token. It does not reserve/change the port or readiness deadline.
func startSiteTCP(tb testing.TB, o proc.Options, addr string) *proc.Process {
	tb.Helper()
	if os.Getenv(startupDiagnosticsEnv) != "1" {
		return proc.StartTCP(tb, o, addr)
	}
	if !filepath.IsAbs(o.Path) && strings.ContainsRune(o.Path, filepath.Separator) {
		path, err := filepath.Abs(o.Path)
		if err != nil {
			tb.Fatal(err)
		}
		o.Path = path
	}
	root := os.Getenv(startupDiagnosticsDirEnv)
	if root == "" {
		var err error
		root, err = os.MkdirTemp("", "e2e-startup-diagnostics-")
		if err != nil {
			tb.Fatalf("startup diagnostics directory: %v", err)
		}
	}
	if err := os.MkdirAll(root, 0700); err != nil {
		tb.Fatalf("startup diagnostics directory: %v", err)
	}
	dir, err := os.MkdirTemp(root, "launch-")
	if err != nil {
		tb.Fatalf("startup diagnostics launch directory: %v", err)
	}
	dir, err = filepath.Abs(dir)
	if err != nil {
		tb.Fatal(err)
	}
	obs := startupObservation{Test: tb.Name(), Executable: o.Path, Site: o.Dir,
		Address: addr, Token: filepath.Base(dir), Started: time.Now().UTC()}
	pidfile := filepath.Join(dir, "pid")
	writeStartupJSON(tb, dir, &obs)
	path, args := o.Path, o.Args
	o.Path = "/bin/sh"
	o.Args = append([]string{"-c", `printf '%s\n' "$$" > "$1" || exit 125; shift; exec "$@"`,
		"e2e-startup", pidfile, path}, args...)
	o.Env = append(slices.Clone(o.Env), "E2E_STARTUP_TOKEN="+obs.Token)
	d := startupDiagnosticTB{TB: tb, fail: func(reason string) {
		obs.Failure = reason
		obs.PID = startupPID(pidfile)
		writeStartupJSON(tb, dir, &obs)
		captureStartupDiagnostics(tb, dir, obs)
	}}
	p := proc.StartTCP(d, o, addr)
	obs.PID, obs.Ready = p.Pid(), time.Now().UTC()
	writeStartupJSON(tb, dir, &obs)
	return p
}

type startupDiagnosticTB struct {
	testing.TB
	fail func(string)
}

func (d startupDiagnosticTB) Fatalf(format string, args ...any) {
	reason := fmt.Sprintf(format, args...)
	d.fail(reason)
	d.TB.Fatalf("%s", reason)
}

func startupPID(path string) int {
	b, _ := os.ReadFile(path)
	pid, _ := strconv.Atoi(strings.TrimSpace(string(b)))
	if pid < 1 {
		return 0
	}
	return pid
}

func writeStartupJSON(tb testing.TB, dir string, obs *startupObservation) {
	b, _ := json.MarshalIndent(obs, "", "  ")
	if err := os.WriteFile(filepath.Join(dir, "startup.json"), append(b, '\n'), 0600); err != nil {
		tb.Logf("startup diagnostics metadata: %v", err)
	}
}

// startupCapBuffer prevents a noisy command from consuming unbounded memory.
type startupCapBuffer struct {
	bytes.Buffer
	limit int
}

func (b *startupCapBuffer) Write(p []byte) (int, error) {
	n := len(p)
	if left := b.limit - b.Len(); left > 0 {
		b.Buffer.Write(p[:min(left, len(p))])
	}
	return n, nil
}

func startupCommand(ctx context.Context, name string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = append(os.Environ(), "LC_ALL=C")
	cmd.WaitDelay = time.Second
	out := &startupCapBuffer{limit: 256 * 1024}
	cmd.Stdout, cmd.Stderr = out, out
	err := cmd.Run()
	return out.Bytes(), err
}

func startupTokenMatches(ctx context.Context, pid int, token string) bool {
	if pid < 1 {
		return false
	}
	want := "E2E_STARTUP_TOKEN=" + token
	if runtime.GOOS == "linux" {
		b, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "environ"))
		return err == nil && slices.Contains(strings.Split(string(b), "\x00"), want)
	}
	var b []byte
	var err error
	switch inspectionOS(runtime.GOOS) {
	case "solaris":
		b, err = startupCommand(ctx, "pargs", "-e", strconv.Itoa(pid))
	case "freebsd":
		b, err = startupCommand(ctx, "procstat", "-e", strconv.Itoa(pid))
	default:
		b, err = startupCommand(ctx, "ps", "eww", "-p", strconv.Itoa(pid), "-o", "command=")
	}
	// Only the boolean escapes: never save or print captured environments.
	return err == nil && hasSiteEnvironment(string(b), want)
}

func captureStartupDiagnostics(tb testing.TB, dir string, obs startupObservation) {
	tb.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	tb.Logf("STARTUP-DIAGNOSTICS before process cleanup: test=%s pid=%d address=%s site=%s artifact=%s", obs.Test, obs.PID, obs.Address, obs.Site, dir)
	run := func(file, name string, args ...string) []byte {
		start := time.Now().UTC()
		b, err := startupCommand(ctx, name, args...)
		header := fmt.Sprintf("started=%s elapsed=%s command=%s %s error=%v\n", start.Format(time.RFC3339Nano), time.Since(start), name, strings.Join(args, " "), err)
		_ = os.WriteFile(filepath.Join(dir, file), append([]byte(header), b...), 0600)
		// Full capped output survives in artifacts; a bounded console copy
		// remains useful if the VM's final file sync fails.
		tb.Logf("STARTUP-DIAGNOSTICS %s: %s%s", file, header, b[:min(len(b), 16384)])
		return b
	}
	ps := run("processes.txt", "ps", processTableArgs(runtime.GOOS)...)
	run("netstat.txt", "netstat", "-an") // numeric: no resolver work
	if runtime.GOOS == "linux" {
		// Ubuntu may have ss but not net-tools; also retain socket ownership.
		run("ss.txt", "ss", "-anp")
	}
	owned := map[int]bool{}
	if obs.PID > 0 {
		owned = processDescendants(parseProcesses(string(ps)), obs.PID)
	}
	var pids []int
	if obs.PID > 0 {
		pids = append(pids, obs.PID)
	}
	for pid := range owned {
		if pid != obs.PID {
			pids = append(pids, pid)
		}
	}
	if len(pids) > 1 {
		slices.Sort(pids[1:])
	}
	for _, pid := range pids[:min(len(pids), 16)] {
		if !startupTokenMatches(ctx, pid, obs.Token) {
			tb.Logf("STARTUP-DIAGNOSTICS pid=%d launch-token mismatch/unavailable; not attaching", pid)
			continue
		}
		if runtime.GOOS == "linux" {
			// No debugger dependency or ptrace-policy changes on hosted ARM.
			run(fmt.Sprintf("threads-%d.txt", pid), "ps", "-L", "-p", strconv.Itoa(pid), "-o", "pid,tid,stat,wchan:32,comm")
			run(fmt.Sprintf("fds-%d.txt", pid), "ls", "-l", filepath.Join("/proc", strconv.Itoa(pid), "fd"))
			continue
		}
		if pid == obs.PID {
			run("ptree.txt", "ptree", strconv.Itoa(pid))
		}
		// No -F: do not steal a debugger's control. These native tools
		// briefly stop only a process validated by this launch's token.
		run(fmt.Sprintf("pstack-%d.txt", pid), "pstack", strconv.Itoa(pid))
		run(fmt.Sprintf("pfiles-%d.txt", pid), "pfiles", strconv.Itoa(pid))
	}
	for _, name := range []string{"hosts", "resolv.conf", "nsswitch.conf"} {
		startupFileTail(tb, filepath.Join("/etc", name), filepath.Join(dir, "system-"+name))
	}
	logdir := filepath.Join(obs.Site, "log")
	count := 0
	_ = filepath.WalkDir(logdir, func(path string, entry os.DirEntry, err error) error {
		if err != nil || count >= 16 {
			return filepath.SkipAll
		}
		if entry.Type().IsRegular() {
			rel, _ := filepath.Rel(logdir, path)
			startupFileTail(tb, path, filepath.Join(dir, "site-log-"+strings.ReplaceAll(rel, string(filepath.Separator), "_")))
			count++
		}
		return nil
	})
}

func startupFileTail(tb testing.TB, path, dest string) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil || !st.Mode().IsRegular() {
		return
	}
	const limit = 64 * 1024
	if st.Size() > limit {
		_, _ = f.Seek(-limit, io.SeekEnd)
	}
	b, err := io.ReadAll(io.LimitReader(f, limit))
	if err == nil {
		_ = os.WriteFile(dest, b, 0600)
		tb.Logf("STARTUP-DIAGNOSTICS file=%s tail=%s", path, b[:min(len(b), 8192)])
	}
}

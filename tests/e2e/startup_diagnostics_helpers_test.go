package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	testkit "github.com/go-while/nntp-testkit"
	"github.com/go-while/nntp-testkit/proc"
)

// A stand-in child for testing a live accepted connection with no greeting.
// This mode is never enabled in a Diablo server or the normal test runner.
func runStartupProbe() bool {
	addr := os.Getenv("E2E_STARTUP_PROBE")
	if addr == "" {
		return false
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	var held []net.Conn
	for {
		c, err := ln.Accept()
		if err != nil {
			return true
		}
		if os.Getenv("E2E_STARTUP_PROBE_GREET") == "1" {
			_, _ = io.WriteString(c, "200 startup probe\r\n")
			c.Close()
		} else {
			held = append(held, c)
		}
	}
}

type startupExpectedFatal struct{}

type startupCaptureTB struct {
	testing.TB
	logs  []string
	fatal string
}

func (tb *startupCaptureTB) Logf(format string, args ...any) {
	tb.logs = append(tb.logs, fmt.Sprintf(format, args...))
}

func (tb *startupCaptureTB) Fatalf(format string, args ...any) {
	tb.fatal = fmt.Sprintf(format, args...)
	panic(startupExpectedFatal{}) // keep the real test green; assertions below
}

func startupProbeOptions(t *testing.T) (proc.Options, string) {
	t.Helper()
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	addr := fmt.Sprintf("127.0.0.1:%d", testkit.FreePort(t))
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "log"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "log", "probe.log"), []byte("diagnostic log marker\n"), 0600); err != nil {
		t.Fatal(err)
	}
	return proc.Options{Path: self, Args: []string{"-test.run=^$"}, Dir: dir,
		Env: []string{"E2E_STARTUP_PROBE=" + addr}, Deadline: 5 * time.Second}, addr
}

func TestStartupDiagnosticsBeforeCleanup(t *testing.T) {
	root := t.TempDir()
	t.Setenv(startupDiagnosticsEnv, "1")
	t.Setenv(startupDiagnosticsDirEnv, root)
	opts, addr := startupProbeOptions(t)
	tb := &startupCaptureTB{TB: t}
	func() {
		defer func() {
			r := recover()
			if _, ok := r.(startupExpectedFatal); !ok {
				t.Errorf("expected intercepted startup Fatalf, got %v", r)
			}
		}()
		startSiteTCP(tb, opts, addr)
	}()
	if !strings.Contains(tb.fatal, "greeting:") || !strings.Contains(strings.Join(tb.logs, "\n"), "before process cleanup") {
		t.Fatalf("startup failure was not captured before Fatalf: %q", tb.fatal)
	}
	dirs, err := os.ReadDir(root)
	if err != nil || len(dirs) != 1 {
		t.Fatalf("diagnostic launches: %v %v", dirs, err)
	}
	dir := filepath.Join(root, dirs[0].Name())
	var obs startupObservation
	b, err := os.ReadFile(filepath.Join(dir, "startup.json"))
	if err != nil || json.Unmarshal(b, &obs) != nil || obs.PID <= 0 || obs.Failure == "" {
		t.Fatalf("startup metadata: %s %v", b, err)
	}
	if err := syscall.Kill(obs.PID, 0); err != nil {
		t.Fatalf("child was killed before the diagnostic assertion: %v", err)
	}
	for _, file := range []string{"netstat.txt", "processes.txt", fmt.Sprintf("pstack-%d.txt", obs.PID), "site-log-probe.log"} {
		if _, err := os.Stat(filepath.Join(dir, file)); err != nil {
			t.Errorf("missing diagnostic %s: %v", file, err)
		}
	}
	// proc's registered cleanup still stops this stand-in after these checks.
}

func TestStartupDiagnosticsDisabled(t *testing.T) {
	root := t.TempDir()
	t.Setenv(startupDiagnosticsEnv, "")
	t.Setenv(startupDiagnosticsDirEnv, root)
	opts, addr := startupProbeOptions(t)
	opts.Env = append(opts.Env, "E2E_STARTUP_PROBE_GREET=1")
	p := startSiteTCP(t, opts, addr)
	p.Stop()
	if entries, err := os.ReadDir(root); err != nil || len(entries) != 0 {
		t.Errorf("disabled diagnostics changed launch/artifacts: %v %v", entries, err)
	}
}

func TestStartupDiagnosticOutputBound(t *testing.T) {
	b := &startupCapBuffer{limit: 8}
	for range 3 {
		if n, err := b.Write([]byte("abcdef")); n != 6 || err != nil {
			t.Fatalf("capped writer stopped consuming output: %d %v", n, err)
		}
	}
	if b.Len() != 8 || b.String() != "abcdefab" {
		t.Errorf("output cap: %q", b.String())
	}
}

func TestStartupShardRepeats(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(fmt.Sprintf("failure=%v", fail), func(t *testing.T) {
			dir := t.TempDir()
			calls := filepath.Join(dir, "calls")
			if err := os.Mkdir(calls, 0700); err != nil {
				t.Fatal(err)
			}
			bin := filepath.Join(dir, "fake-test")
			script := `#!/bin/sh
case "$*" in
    *-test.list*) printf '%s\n' TestStartupFakeA TestStartupFakeB; exit 0 ;;
esac
printf '%s\n' "$E2E_STARTUP_DIAGNOSTICS" "$E2E_STARTUP_DIAGNOSTICS_DIR" "$NNTP_TEST_DEADLINE" "$*" > "$E2E_FAKE_CALLS/$$"
[ "${E2E_FAKE_FAIL:-0}" = 0 ]
`
			if err := os.WriteFile(bin, []byte(script), 0700); err != nil {
				t.Fatal(err)
			}
			outdir := filepath.Join(dir, "results")
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, "sh", "ci/repeat-startup-shards", bin, outdir, "2", "-test.count=1", "-test.timeout=1200s")
			cmd.Env = append(os.Environ(), "E2E_FAKE_CALLS="+calls, fmt.Sprintf("E2E_FAKE_FAIL=%d", boolInt(fail)), "NNTP_TEST_DEADLINE=30s")
			out, err := cmd.CombinedOutput()
			if (err != nil) != fail {
				t.Fatalf("repeat runner: %v\n%s", err, out)
			}
			b, err := os.ReadFile(filepath.Join(outdir, "results.tsv"))
			if err != nil || len(strings.Split(strings.TrimSpace(string(b)), "\n")) != 3 {
				t.Fatalf("did not retain both iterations: %s %v", b, err)
			}
			entries, err := os.ReadDir(calls)
			if err != nil || len(entries) != 4 {
				t.Fatalf("two shards per iteration: %v %v", entries, err)
			}
			for _, entry := range entries {
				b, _ := os.ReadFile(filepath.Join(calls, entry.Name()))
				if !strings.HasPrefix(string(b), "1\n") || !strings.Contains(string(b), "30s\n") || !strings.Contains(string(b), "-test.timeout=1200s") {
					t.Errorf("runner changed diagnostics/deadlines: %s", b)
				}
			}
		})
	}
}

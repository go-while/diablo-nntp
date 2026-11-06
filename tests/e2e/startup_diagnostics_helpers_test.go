package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"runtime"
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
	files := []string{"netstat.txt", "processes.txt", "site-log-probe.log"}
	if runtime.GOOS == "linux" {
		files = append(files, "ss.txt", fmt.Sprintf("threads-%d.txt", obs.PID), fmt.Sprintf("fds-%d.txt", obs.PID))
	} else {
		files = append(files, fmt.Sprintf("pstack-%d.txt", obs.PID))
	}
	for _, file := range files {
		if _, err := os.Stat(filepath.Join(dir, file)); err != nil {
			t.Errorf("missing diagnostic %s: %v", file, err)
		}
	}
	if startupTokenMatches(context.Background(), obs.PID, obs.Token+"-wrong") {
		t.Error("accepted another launch's token")
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

func TestStartupDiagnosticsReady(t *testing.T) {
	root := t.TempDir()
	t.Setenv(startupDiagnosticsEnv, "1")
	t.Setenv(startupDiagnosticsDirEnv, root)
	opts, addr := startupProbeOptions(t)
	opts.Env = append(opts.Env, "E2E_STARTUP_PROBE_GREET=1")
	opts.Ulimit = []string{"-n 128"}
	p := startSiteTCP(t, opts, addr)
	defer p.Stop()
	paths, err := filepath.Glob(filepath.Join(root, "launch-*", "startup.json"))
	if err != nil || len(paths) != 1 {
		t.Fatalf("ready metadata: %v %v", paths, err)
	}
	b, err := os.ReadFile(paths[0])
	var obs startupObservation
	if err != nil || json.Unmarshal(b, &obs) != nil || obs.PID != p.Pid() || obs.Ready.IsZero() || obs.Failure != "" {
		t.Fatalf("ready metadata: %s %v", b, err)
	}
	if !startupTokenMatches(context.Background(), obs.PID, obs.Token) {
		t.Error("exec launcher did not preserve the PID/token through Ulimit")
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

package e2e

// D-307: dclient -t SECONDS gives up when the server sends nothing for that long while dclient
// waits for the greeting or a reply ("No data from server for N seconds", exit 2). Without -t
// it waits as before. Fails on main: there is no -t (usage, exit 1).

import (
	"bufio"
	"bytes"
	"context"
	"io"
	"net"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// dclientScriptServer accepts one connection on 127.0.0.1 and plays script: each step reads one
// command line (if cmd is not "") and then writes reply after delay. After the last step it
// sends nothing and keeps the connection open until the test ends. It returns the port.
func dclientScriptServer(t *testing.T, script []struct {
	cmd, reply string
	delay      time.Duration
}) int {
	t.Helper()
	ln, err := net.ListenTCP("tcp", &net.TCPAddr{IP: net.ParseIP("127.0.0.1")})
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	t.Cleanup(func() { close(done); ln.Close() })
	go func() {
		c, err := ln.AcceptTCP()
		if err != nil {
			return
		}
		defer c.Close()
		r := bufio.NewReader(c)
		for _, step := range script {
			if step.cmd != "" {
				line, err := r.ReadString('\n')
				if err != nil || line != step.cmd+"\r\n" {
					return
				}
			}
			time.Sleep(step.delay)
			if _, err := io.WriteString(c, step.reply); err != nil {
				return
			}
		}
		<-done
	}()
	return ln.Addr().(*net.TCPAddr).Port
}

func TestDclientReadTimeout(t *testing.T) {
	type step = struct {
		cmd, reply string
		delay      time.Duration
	}
	const greeting = "200 ready\r\n"
	const date = "111 20261126000000\r\n"
	for _, tc := range []struct {
		name   string
		script []step
		args   []string
		exit   int
		out    string // the output must contain this
		min    time.Duration
	}{
		// nothing at all after the connect
		{"greeting", nil, []string{"-t", "1"}, 2, "No data from server for 1 seconds\n", 900 * time.Millisecond},
		// the greeting, then no reply to DATE
		{"reply", []step{{"", greeting, 0}}, []string{"-t", "1"}, 2, "No data from server for 1 seconds\n", 900 * time.Millisecond},
		// DATE answered, then no reply to QUIT
		{"quit", []step{{"", greeting, 0}, {"DATE", date, 0}}, []string{"-t", "1"}, 2, date + "No data from server for 1 seconds\n", 900 * time.Millisecond},
		// the limit is per read, not for the whole session: two replies 1.2 s apart with -t 2
		{"slow", []step{{"", greeting, 0}, {"DATE", date, 1200 * time.Millisecond}, {"QUIT", "205 bye\r\n", 1200 * time.Millisecond}},
			[]string{"-t", "2"}, 0, date, 2 * time.Second},
	} {
		t.Run(tc.name, func(t *testing.T) {
			port := dclientScriptServer(t, tc.script)
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			args := append(append([]string{}, tc.args...), "-p", strconv.Itoa(port), "127.0.0.1", "DATE")
			cmd := exec.CommandContext(ctx, filepath.Join(bin(t), "dclient"), args...)
			var out bytes.Buffer
			cmd.Stdout, cmd.Stderr = &out, &out
			cmd.WaitDelay = time.Second
			start := time.Now()
			cmd.Run()
			took := time.Since(start)
			if cmd.ProcessState == nil || cmd.ProcessState.ExitCode() != tc.exit || !strings.Contains(out.String(), tc.out) {
				t.Fatalf("dclient %v: exit %v after %v, want %d; output %q, want it to contain %q",
					args, cmd.ProcessState, took, tc.exit, out.String(), tc.out)
			}
			if took < tc.min || took > 10*time.Second {
				t.Fatalf("dclient %v took %v, want at least %v (and well under the test's 15 s)", args, took, tc.min)
			}
		})
	}

	// Without -t (and with -t 0) dclient still waits: after 3 s of silence it is still running.
	for _, args := range [][]string{nil, {"-t", "0"}} {
		t.Run("no-limit"+strings.Join(args, ""), func(t *testing.T) {
			port := dclientScriptServer(t, []step{{"", greeting, 0}})
			cmd := exec.Command(filepath.Join(bin(t), "dclient"), append(append([]string{}, args...), "-p", strconv.Itoa(port), "127.0.0.1", "DATE")...)
			var out bytes.Buffer
			cmd.Stdout, cmd.Stderr = &out, &out
			if err := cmd.Start(); err != nil {
				t.Fatal(err)
			}
			exited := make(chan error, 1)
			go func() { exited <- cmd.Wait() }()
			select {
			case err := <-exited:
				t.Fatalf("dclient %v exited after less than 3 s of silence (%v): %q", args, err, out.String())
			case <-time.After(3 * time.Second):
			}
			cmd.Process.Kill()
			<-exited
		})
	}

	// A bad -t is a usage error (exit 1); no connection is made.
	for _, v := range []string{"x", "1x", "", "-1", "86401", "99999999999999999999"} {
		t.Run("bad"+v, func(t *testing.T) {
			cmd := exec.Command(filepath.Join(bin(t), "dclient"), "-t", v, "-p", "1", "127.0.0.1", "DATE")
			out, _ := cmd.CombinedOutput()
			if cmd.ProcessState == nil || cmd.ProcessState.ExitCode() != 1 || !strings.Contains(string(out), "Usage:") {
				t.Fatalf("dclient -t %q: exit %v, output %q; want usage, exit 1", v, cmd.ProcessState, out)
			}
		})
	}
}

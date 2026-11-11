package e2e

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"

	testkit "github.com/go-while/nntp-testkit"
)

// TestDeepRevLogProgNoPipe (L8-07): a log pattern "|prog" (path_generallog, path_incominglog,
// path_postlog, ...) is opened by openProg() (lib/log.c) with pipe() and fork(). When one of
// them failed (no descriptor left, no process slot) openProg() returned with the log's FILE
// still NULL: the caller then wrote the line to a NULL FILE, and for the general log the
// report of the failure went through logit(), back into openProg(), without end.
//
// Here dpath reads a diablo.config with the general log on a program and an unknown
// directive; the directive is logged while the config file is open, and "ulimit -n 5" leaves
// one free descriptor at that moment, so pipe() fails with EMFILE. dpath must report the
// directive and exit 1 as for any bad config, not die of a signal (SIGSEGV after the stack
// is used up).
func TestDeepRevLogProgNoPipe(t *testing.T) {
	s := NewSite(t, SiteOptions{NoStart: true, Config: map[string]string{
		"path_generallog": "|/bin/cat",
		"zzunknown":       "1", // sorts behind path_generallog
	}})

	ctx, cancel := context.WithTimeout(context.Background(), testkit.Deadline())
	defer cancel()
	cmd := exec.CommandContext(ctx, "/bin/sh", "-c", `ulimit -n 5 || exit 99; exec "$0" home`, filepath.Join(s.Bin, "dpath"))
	cmd.Dir = s.Dir
	cmd.Env = append(os.Environ(), s.Env...)
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	err := cmd.Run()

	var ee *exec.ExitError
	if !errors.As(err, &ee) {
		t.Fatalf("dpath with an unknown directive: err=%v, want exit status 1; output:\n%s", err, out.String())
	}
	if ws, ok := ee.Sys().(syscall.WaitStatus); ok && ws.Signaled() {
		t.Fatalf("dpath died of signal %v when pipe() failed for the \"|prog\" general log; output:\n%s", ws.Signal(), out.String())
	}
	if ee.ExitCode() != 1 {
		t.Fatalf("dpath: exit status %d, want 1; output:\n%s", ee.ExitCode(), out.String())
	}
	if !bytes.Contains(out.Bytes(), []byte("Unknown diablo.config command: zzunknown")) {
		t.Errorf("dpath did not report the unknown directive; output:\n%s", out.String())
	}
}

// syslogFileSource records syslog() and vsyslog() calls (and their _FORTIFY_SOURCE forms) as
// lines in the file SYSLOG_FILE instead of sending them to the system log.
const syslogFileSource = `#define _GNU_SOURCE
#define __EXTENSIONS__
#include <fcntl.h>
#include <stdarg.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <syslog.h>
#include <unistd.h>

static void
put(const char *fmt, va_list va)
{
    const char *f = getenv("SYSLOG_FILE");
    char buf[2048];
    int fd, n;

    if (f == NULL)
	return;
    n = vsnprintf(buf, sizeof(buf) - 1, fmt, va);
    if (n < 0)
	return;
    if (n > (int)sizeof(buf) - 2)
	n = sizeof(buf) - 2;
    buf[n++] = '\n';
    if ((fd = open(f, O_WRONLY|O_CREAT|O_APPEND, 0644)) >= 0) {
	(void)write(fd, buf, n);
	close(fd);
    }
}

void syslog(int pri, const char *fmt, ...) { va_list va; (void)pri; va_start(va, fmt); put(fmt, va); va_end(va); }
void vsyslog(int pri, const char *fmt, va_list va) { (void)pri; put(fmt, va); }
void __syslog_chk(int pri, int flag, const char *fmt, ...) { va_list va; (void)pri; (void)flag; va_start(va, fmt); put(fmt, va); va_end(va); }
void __vsyslog_chk(int pri, int flag, const char *fmt, va_list va) { (void)pri; (void)flag; put(fmt, va); }
`

// TestDeepRevLogProgNotStarted (Sol, second review of L8-07): when the program of a "|prog"
// log cannot be started, the child's execl() fails after the fork. The parent did not learn
// of it: it had opened its end of the pipe, wrote the log lines into it for nobody (or died
// of SIGPIPE once the child was gone) and never fell back to syslog. The child now reports
// the failure through a second pipe and the parent falls back.
//
// dpath reads a diablo.config with the general log on a program that does not exist and an
// unknown directive. The report of the directive must reach syslog (a preloaded syslog()
// writes it to a file), and dpath must exit 1 as for any bad config.
func TestDeepRevLogProgNotStarted(t *testing.T) {
	so := preloadShim(t, "syslogfile", syslogFileSource)
	logf := filepath.Join(t.TempDir(), "syslog")
	s := NewSite(t, SiteOptions{NoStart: true, Config: map[string]string{
		"path_generallog": "|" + filepath.Join(t.TempDir(), "no-such-program"),
		"zzunknown":       "1", // sorts behind path_generallog
	}})

	ctx, cancel := context.WithTimeout(context.Background(), testkit.Deadline())
	defer cancel()
	cmd := exec.CommandContext(ctx, filepath.Join(s.Bin, "dpath"), "home")
	cmd.Dir = s.Dir
	cmd.Env = append(append(os.Environ(), s.Env...), lockFaultEnv(so), "SYSLOG_FILE="+logf)
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	err := cmd.Run()

	var ee *exec.ExitError
	if !errors.As(err, &ee) {
		t.Fatalf("dpath with an unknown directive: err=%v, want exit status 1; output:\n%s", err, out.String())
	}
	if ws, ok := ee.Sys().(syscall.WaitStatus); ok && ws.Signaled() {
		t.Fatalf("dpath died of signal %v writing to a log program that never started; output:\n%s", ws.Signal(), out.String())
	}
	sl, _ := os.ReadFile(logf)
	if !bytes.Contains(sl, []byte("unable to execl")) {
		t.Fatalf("the scenario did not develop: no failed execl() of the log program in syslog:\n%s", sl)
	}
	if !bytes.Contains(sl, []byte("Unknown diablo.config command: zzunknown")) {
		t.Errorf("the report of the unknown directive did not reach syslog after the log program failed to start; syslog:\n%s\noutput:\n%s", sl, out.String())
	}
}

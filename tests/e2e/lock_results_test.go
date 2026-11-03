package e2e

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"
)

// TestHistoryReadOnlyWaitsForCreator: a program that opens the history
// read-only (dilookup, dreadart, didump -f) and finds it not yet valid
// waits on the lock of the process that creates it, then reads it again.
// The test holds that lock (bytes 0-3, as hflock() takes it) on an empty
// file, lets dilookup wait, writes a valid history and releases the lock.
// Fails before the fix: the wait asked for an exclusive lock on the
// read-only descriptor, which fails at once (EBADF), and dilookup exited
// with "Unable to create ... read-only mode".
func TestHistoryReadOnlyWaitsForCreator(t *testing.T) {
	s := NewSite(t, SiteOptions{NoStart: true})
	dir := t.TempDir()
	ref := filepath.Join(dir, "ref")
	if out, err := s.TryRun("dhisctl", ref); err != nil && !fileExists(ref) {
		t.Fatalf("dhisctl %s: %v\n%s", ref, err, out)
	}
	valid, err := os.ReadFile(ref)
	if err != nil || len(valid) == 0 {
		t.Fatalf("no history from dhisctl: %v", err)
	}

	hist := filepath.Join(dir, "dhistory")
	f, err := os.OpenFile(hist, os.O_RDWR|os.O_CREATE, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	lk := syscall.Flock_t{Type: syscall.F_WRLCK, Whence: 0, Start: 0, Len: 4}
	if err := syscall.FcntlFlock(f.Fd(), syscall.F_SETLK, &lk); err != nil {
		t.Fatalf("lock %s: %v", hist, err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, filepath.Join(s.Bin, "dilookup"), "-h", hist, "<none@e2e.test>")
	cmd.Dir = s.Dir
	cmd.Env = append(os.Environ(), s.Env...)
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	if err := waitLockWaiter(cmd.Process.Pid, done); err != nil {
		t.Fatalf("dilookup did not wait for the history creator: %v\n%s", err, out.String())
	}
	if _, err := f.WriteAt(valid, 0); err != nil {
		t.Fatal(err)
	}
	lk.Type = syscall.F_UNLCK
	if err := syscall.FcntlFlock(f.Fd(), syscall.F_SETLK, &lk); err != nil {
		t.Fatal(err)
	}
	<-done // not found is a non-zero exit
	if o := out.String(); strings.Contains(o, "Unable to create") ||
		strings.Contains(o, "Incompatible") || strings.Contains(o, "lock") {
		t.Errorf("dilookup did not read the history its creator finished:\n%s", o)
	}
}

// TestDoutqRunning: doutq lists the queue files a dnewslink works on, which
// it sees by the write lock dnewslink holds on all of the file. The test
// holds such a lock on one of three files: exactly that one is listed, also
// when the queue files are read-only for the user running doutq. Before the
// fix doutq took the lock itself on a descriptor opened for writing: a
// dnewslink starting on the file in that moment skipped it, and on read-only
// queue files the open failed and nothing was listed (fails on main).
func TestDoutqRunning(t *testing.T) {
	s := NewSite(t, SiteOptions{NoStart: true, Newsfeeds: GlobalLabel + `label peer
    inhost ` + PeerIP + `
    hostname 127.0.0.1
    addgroup *
end
`})
	s.WriteFile(t, "dqueue/.peer.seq", "0 3\n")
	var files []string
	for i := 0; i < 3; i++ {
		rel := fmt.Sprintf("dqueue/peer.S%05d", i)
		s.WriteFile(t, rel, "D.00000001/B.0001 <q@e2e.test> 0,100\n")
		files = append(files, filepath.Join(s.Dir, rel))
	}
	running := func() string {
		t.Helper()
		out := s.Run(t, "doutq", "peer")
		for _, l := range strings.Split(out, "\n") {
			if strings.HasPrefix(l, "peer ") {
				_, list, _ := strings.Cut(l, "\t")
				return strings.TrimSpace(list)
			}
		}
		t.Fatalf("no line for peer in doutq output:\n%s", out)
		return ""
	}

	if got := running(); got != "" {
		t.Errorf("doutq with no lock held: running %q, want none", got)
	}

	f, err := os.OpenFile(files[1], os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	lk := syscall.Flock_t{Type: syscall.F_WRLCK, Whence: 0, Start: 0, Len: 0}
	if err := syscall.FcntlFlock(f.Fd(), syscall.F_SETLK, &lk); err != nil {
		t.Fatalf("lock %s: %v", files[1], err)
	}
	if got := running(); got != "00001" {
		t.Errorf("doutq with peer.S00001 locked: running %q, want 00001", got)
	}

	if os.Geteuid() == 0 {
		t.Log("root opens read-only files for writing: read-only case skipped")
		return
	}
	for _, p := range files {
		if err := os.Chmod(p, 0o444); err != nil {
			t.Fatal(err)
		}
	}
	if got := running(); got != "00001" {
		t.Errorf("doutq on read-only queue files with peer.S00001 locked: running %q, want 00001", got)
	}
}

// TestDexpireoverLockTimeoutKept: the blocking lock wrappers now start a wait
// again when a signal interrupts it, except where the caller asks a signal
// to end it. dexpireover gives up on a group's over.* lock after
// LockWaitTime (an alarm); the test holds that lock, and dexpireover -s must
// still give up on the resize and finish instead of waiting for ever.
func TestDexpireoverLockTimeoutKept(t *testing.T) {
	s, home := overSite(t, map[string]string{"readergrouphash": "hierarchy"})
	s.Run(t, "dgrpctl", "adjust", "e2e.test", "NE", "=1000")
	s.WriteFile(t, "dexpire.ctl", "e2e.*:x30\n")
	over := filepath.Join(home, "e2e", "o.0.e2e.test")
	index := append(overHeadMax("e2e.test", 500), make([]byte, 500*32)...)
	writeOver(t, over, index)

	f, err := os.OpenFile(over, os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	lk := syscall.Flock_t{Type: syscall.F_WRLCK, Whence: 0, Start: 4, Len: 4}
	if err := syscall.FcntlFlock(f.Fd(), syscall.F_SETLK, &lk); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, filepath.Join(s.Bin, "dexpireover"), "-a", "-s", "-y", "-v")
	cmd.Dir = s.Dir
	cmd.Env = append(os.Environ(), s.Env...)
	out, err := cmd.CombinedOutput()
	if ctx.Err() != nil {
		t.Fatalf("dexpireover still waited for the over.* lock after 60 s:\n%s", out)
	}
	if err != nil {
		t.Fatalf("dexpireover: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "file in use") {
		t.Errorf("dexpireover did not give up on the held over.* lock:\n%s", out)
	}
	if b, err := os.ReadFile(over); err != nil || !bytes.Equal(b, index) {
		t.Errorf("dexpireover changed the over file it could not lock: %v\n%s", err, out)
	}
}

// lockFaultShim builds an LD_PRELOAD library that changes blocking fcntl
// locks (F_SETLKW, not unlocks): LOCKFAULT=eintr fails the first with EINTR,
// LOCKFAULT=enolck fails all with ENOLCK. With LOCKFAULT_LOG each such call
// is logged as "W|R start len".
func lockFaultShim(t *testing.T) string {
	t.Helper()
	if runtime.GOOS != "linux" {
		t.Skip("the lock fault injector uses LD_PRELOAD on Linux")
	}
	cc := wmCC(t)
	dir := t.TempDir()
	c := filepath.Join(dir, "lockfault.c")
	so := filepath.Join(dir, "lockfault.so")
	if err := os.WriteFile(c, []byte(lockFaultSource), 0o600); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command(cc, "-shared", "-fPIC", "-o", so, c, "-ldl").CombinedOutput(); err != nil {
		t.Fatalf("build fault injector: %v\n%s", err, out)
	}
	return so
}

const lockFaultSource = `#define _GNU_SOURCE
#include <dlfcn.h>
#include <errno.h>
#include <fcntl.h>
#include <stdarg.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>

typedef int (*fcntl_fn)(int, int, ...);

static int
fault(int cmd, void *arg)
{
    static int calls;
    struct flock *fl = arg;
    const char *mode = getenv("LOCKFAULT");
    const char *log = getenv("LOCKFAULT_LOG");

    if (cmd != F_SETLKW || mode == NULL || fl->l_type == F_UNLCK)
	return 0;
    if (log) {
	FILE *f = fopen(log, "a");
	if (f) {
	    fprintf(f, "%c %lld %lld\n", fl->l_type == F_WRLCK ? 'W' : 'R',
		(long long)fl->l_start, (long long)fl->l_len);
	    fclose(f);
	}
    }
    if (strcmp(mode, "eintr") == 0 && calls++ == 0)
	return EINTR;
    if (strcmp(mode, "enolck") == 0)
	return ENOLCK;
    return 0;
}

static int
call(const char *name, int fd, int cmd, void *arg)
{
    fcntl_fn real = (fcntl_fn)dlsym(RTLD_NEXT, name);
    int e = fault(cmd, arg);

    if (e) {
	errno = e;
	return -1;
    }
    if (real == NULL)
	real = (fcntl_fn)dlsym(RTLD_NEXT, "fcntl");
    return real(fd, cmd, arg);
}

int
fcntl(int fd, int cmd, ...)
{
    va_list va;
    void *arg;

    va_start(va, cmd);
    arg = va_arg(va, void *);
    va_end(va);
    return call("fcntl", fd, cmd, arg);
}

int
fcntl64(int fd, int cmd, ...)
{
    va_list va;
    void *arg;

    va_start(va, cmd);
    arg = va_arg(va, void *);
    va_end(va);
    return call("fcntl64", fd, cmd, arg);
}
`

// newKPDB writes a KP database with group e2e.test (NB and NE) using dkp.
func newKPDB(t *testing.T, dkp string) string {
	t.Helper()
	db := filepath.Join(t.TempDir(), "active.kp")
	for _, a := range [][]string{
		{"-w", db, "e2e.test", "NB=0000000001"},
		{"-w", db, "e2e.test", "NE=0000000300"},
	} {
		if out, err := exec.Command(dkp, a...).CombinedOutput(); err != nil {
			t.Fatalf("dkp %s: %v\n%s", strings.Join(a, " "), err, out)
		}
	}
	return db
}

// TestKPDBLockInterrupted: a blocking lock that a signal interrupts (EINTR)
// is waited for again. The injector fails dkp's first blocking lock (the
// open lock at offset 4) with EINTR. Fails before the fix: the lock was not
// asked for again, so the write went on without it.
func TestKPDBLockInterrupted(t *testing.T) {
	so := lockFaultShim(t)
	dkp := filepath.Join(bin(t), "dkp")
	db := newKPDB(t, dkp)
	log := filepath.Join(t.TempDir(), "locks")

	cmd := exec.Command(dkp, "-w", db, "e2e.test", "NE=0000000301")
	cmd.Env = append(os.Environ(), "LD_PRELOAD="+so, "LOCKFAULT=eintr", "LOCKFAULT_LOG="+log)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("dkp -w: %v\n%s", err, out)
	}
	b, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	calls := strings.Split(strings.TrimSpace(string(b)), "\n")
	if len(calls) < 2 || calls[0] != "W 4 4" || calls[1] != "W 4 4" {
		t.Errorf("the interrupted open lock was not asked for again; blocking locks:\n%s", b)
	}
	if live, _, _ := kpRecords(t, db, "e2e.test"); len(live) != 1 || !strings.Contains(live[0], "NE=0000000301") {
		t.Errorf("record after dkp -w: %q", live)
	}
}

// TestKPDBLockFailureStops: when a record lock or the append lock of a KP
// database cannot be taken (ENOLCK from the injector), a write stops with an
// error and leaves the file alone. Fails before the fix: the result was not
// checked and the write went on without the lock (where two writers can hand
// out one article number twice), and dkp exited 0.
func TestKPDBLockFailureStops(t *testing.T) {
	so := lockFaultShim(t)
	dkp := filepath.Join(bin(t), "dkp")
	for _, tc := range []struct {
		name, key, tokval, want string
	}{
		{"record", "e2e.test", "NE=0000000301", "record lock failed"},
		{"append", "e2e.new", "NB=0000000001", "append lock failed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := newKPDB(t, dkp)
			before, err := os.ReadFile(db)
			if err != nil {
				t.Fatal(err)
			}
			cmd := exec.Command(dkp, "-w", db, tc.key, tc.tokval)
			cmd.Env = append(os.Environ(), "LD_PRELOAD="+so, "LOCKFAULT=enolck")
			out, err := cmd.CombinedOutput()
			if err == nil {
				t.Errorf("dkp -w %s %s exited 0 without its lock:\n%s", tc.key, tc.tokval, out)
			}
			if !strings.Contains(string(out), tc.want) {
				t.Errorf("dkp did not report %q:\n%s", tc.want, out)
			}
			if after, _ := os.ReadFile(db); !bytes.Equal(before, after) {
				t.Errorf("dkp changed the database without its lock:\n%s", after)
			}
		})
	}
}

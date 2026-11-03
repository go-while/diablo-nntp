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

// TestHistoryFastCreatorKeepsLock: a fast-mode creator (diload -f; also
// diloadfromspool -f and dhisexpire on dhistory.new) keeps the creation lock
// for its fast-mode lock. It used to release it and take it again with
// XLOCK_NB; a read-only opener waiting for the creation (here the test, with
// a shared lock) got in between and the creator exited with "Unable to lock
// history file in fast mode". The LD_PRELOAD shim sleeps around that unlock
// so the test waits on the creation lock and wins the gap every time.
// Fails on a7cfecc (the first version of this branch), where a reader could
// wait, and on main, where only the test could (a reader got EBADF, see
// TestHistoryReadOnlyWaitsForCreator).
func TestHistoryFastCreatorKeepsLock(t *testing.T) {
	so := lockFaultShim(t)
	s := NewSite(t, SiteOptions{NoStart: true})
	hist := filepath.Join(t.TempDir(), "dhistory.new")

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, filepath.Join(s.Bin, "diload"), "-f", "-h", "1m", hist)
	cmd.Dir = s.Dir
	cmd.Env = append(append(os.Environ(), s.Env...), "LD_PRELOAD="+so, "LOCKFAULT=unlocksleep")
	cmd.Stdin = strings.NewReader("")
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	// wait until diload holds the creation lock on bytes 0-3
	var f *os.File
	held := false
	for deadline := time.Now().Add(30 * time.Second); !held && time.Now().Before(deadline); {
		select {
		case err := <-done:
			t.Fatalf("diload exited before the test saw its creation lock: %v\n%s", err, out.String())
		default:
		}
		if f == nil {
			f, _ = os.Open(hist)
		}
		if f != nil {
			lk := syscall.Flock_t{Type: syscall.F_RDLCK, Whence: 0, Start: 0, Len: 4}
			if syscall.FcntlFlock(f.Fd(), syscall.F_GETLK, &lk) == nil &&
				lk.Type == syscall.F_WRLCK && int(lk.Pid) == cmd.Process.Pid {
				held = true
				break
			}
		}
		time.Sleep(time.Millisecond)
	}
	if !held {
		t.Fatalf("diload never held the creation lock\n%s", out.String())
	}
	defer f.Close()

	// wait for the creation with a shared lock, as a read-only opener does
	got := make(chan error, 1)
	go func() {
		lk := syscall.Flock_t{Type: syscall.F_RDLCK, Whence: 0, Start: 0, Len: 4}
		got <- syscall.FcntlFlock(f.Fd(), syscall.F_SETLKW, &lk)
	}()
	err := <-done
	if err != nil || strings.Contains(out.String(), "Unable to lock") {
		t.Errorf("diload -f lost its history lock to a waiting reader: %v\n%s", err, out.String())
	}
	select {
	case <-got:
	case <-time.After(30 * time.Second):
		t.Error("the shared lock was not granted after diload exited")
	}
}

// TestDoutqRunning: doutq lists the queue files a dnewslink works on, which
// it sees by the write lock dnewslink holds on all of the file. The test
// holds such a lock on one of three files: exactly that one is listed.
// Before the fix doutq took the lock itself (F_SETLK on a descriptor opened
// for writing): a dnewslink starting on the file in that moment skipped it,
// and on queue files the user can only read the open failed and nothing was
// listed. The subtests fail on main: NoLockTaken (with an LD_PRELOAD shim
// that logs lock calls, also as root) and ReadOnlyQueue (not as root).
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
	running := func(t *testing.T, env ...string) string {
		t.Helper()
		cmd := exec.Command(filepath.Join(s.Bin, "doutq"), "peer")
		cmd.Dir = s.Dir
		cmd.Env = append(append(os.Environ(), s.Env...), env...)
		b, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("doutq peer: %v\n%s", err, b)
		}
		for _, l := range strings.Split(string(b), "\n") {
			if strings.HasPrefix(l, "peer ") {
				_, list, _ := strings.Cut(l, "\t")
				return strings.TrimSpace(list)
			}
		}
		t.Fatalf("no line for peer in doutq output:\n%s", b)
		return ""
	}

	if got := running(t); got != "" {
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
	if got := running(t); got != "00001" {
		t.Errorf("doutq with peer.S00001 locked: running %q, want 00001", got)
	}

	t.Run("NoLockTaken", func(t *testing.T) {
		so := lockFaultShim(t)
		log := filepath.Join(t.TempDir(), "setlocks")
		if got := running(t, "LD_PRELOAD="+so, "LOCKFAULT_SETLOG="+log); got != "00001" {
			t.Errorf("doutq with peer.S00001 locked: running %q, want 00001", got)
		}
		b, _ := os.ReadFile(log)
		for _, l := range strings.Split(string(b), "\n") {
			if strings.Contains(l, "/dqueue/peer.S") {
				t.Errorf("doutq set a lock on a queue file: %s", l)
			}
		}
	})

	t.Run("ReadOnlyQueue", func(t *testing.T) {
		if os.Geteuid() == 0 {
			t.Skip("root opens read-only files for writing")
		}
		for _, p := range files {
			if err := os.Chmod(p, 0o444); err != nil {
				t.Fatal(err)
			}
		}
		if got := running(t); got != "00001" {
			t.Errorf("doutq on read-only queue files with peer.S00001 locked: running %q, want 00001", got)
		}
	})
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

// lockFaultShim builds an LD_PRELOAD library (Linux) for fcntl locks.
//
//	LOCKFAULT=eintr        fail the first blocking lock (F_SETLKW) with EINTR
//	LOCKFAULT=enolck       fail every blocking lock with ENOLCK
//	LOCKFAULT=unlocksleep  sleep 300 ms before and after unlocking bytes 0-3
//	LOCKFAULT_FILE=s       eintr and enolck only for files whose path holds s
//	LOCKFAULT_LOG=f        log each blocking lock (not unlocks) as "W|R start len"
//	LOCKFAULT_SETLOG=f     log every F_SETLK/F_SETLKW as "cmd type start len path"
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
#include <unistd.h>

typedef int (*fcntl_fn)(int, int, ...);

static void
logline(const char *env, const char *line)
{
    const char *p = getenv(env);
    FILE *f;

    if (p && (f = fopen(p, "a")) != NULL) {
	fputs(line, f);
	fclose(f);
    }
}

static int
fault(int fd, int cmd, struct flock *fl)
{
    static int calls;
    const char *mode = getenv("LOCKFAULT");
    const char *only = getenv("LOCKFAULT_FILE");
    char link[64], path[4096], line[4200];
    ssize_t n;

    if (cmd != F_SETLK && cmd != F_SETLKW)
	return 0;
    snprintf(link, sizeof(link), "/proc/self/fd/%d", fd);
    n = readlink(link, path, sizeof(path) - 1);
    path[n > 0 ? n : 0] = 0;
    snprintf(line, sizeof(line), "%s %c %lld %lld %s\n",
	cmd == F_SETLKW ? "SETLKW" : "SETLK",
	fl->l_type == F_WRLCK ? 'W' : fl->l_type == F_RDLCK ? 'R' : 'U',
	(long long)fl->l_start, (long long)fl->l_len, path);
    logline("LOCKFAULT_SETLOG", line);

    if (cmd != F_SETLKW || mode == NULL || fl->l_type == F_UNLCK)
	return 0;
    if (only && strstr(path, only) == NULL)
	return 0;
    snprintf(line, sizeof(line), "%c %lld %lld\n", fl->l_type == F_WRLCK ? 'W' : 'R',
	(long long)fl->l_start, (long long)fl->l_len);
    logline("LOCKFAULT_LOG", line);
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
    const char *mode = getenv("LOCKFAULT");
    struct flock *fl = arg;
    int slow = 0;
    int e = fault(fd, cmd, arg);
    int r;

    if (e) {
	errno = e;
	return -1;
    }
    if (real == NULL)
	real = (fcntl_fn)dlsym(RTLD_NEXT, "fcntl");
    if ((cmd == F_SETLK || cmd == F_SETLKW) && mode &&
	strcmp(mode, "unlocksleep") == 0 && fl->l_type == F_UNLCK &&
	fl->l_start == 0 && fl->l_len == 4)
	slow = 1;
    if (slow)
	usleep(300000);
    r = real(fd, cmd, arg);
    if (slow) {
	e = errno;
	usleep(300000);
	errno = e;
    }
    return r;
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

// TestKPDBReadLockFailureKeepsReader: a dreaderd command that only reads a
// group's record (GROUP) goes on when the record lock fails (ENOLCK from the
// shim, for dactive.kp only): the worst case is a torn read. The first
// version of this branch (a7cfecc) made every record lock failure fatal, so
// GROUP ended the reader process and its connections. On main the result
// was not checked at all, so this passes there.
func TestKPDBReadLockFailureKeepsReader(t *testing.T) {
	so := lockFaultShim(t)
	s := NewReaderSite(t, ReaderOptions{SpoolAddr: "127.0.0.1:1", NoStart: true})
	s.Env = append(s.Env, "LD_PRELOAD="+so, "LOCKFAULT=enolck", "LOCKFAULT_FILE=dactive.kp")
	_, port, _ := strings.Cut(s.Addr, ":")
	startReader(t, s, "[127.0.0.1]:"+port)

	r := s.Reader(t)
	for i := 0; i < 2; i++ {
		expect(t, r, 211, "GROUP e2e.test")
	}
	expect(t, r, 205, "QUIT")
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

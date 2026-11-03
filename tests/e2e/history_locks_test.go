package e2e

import (
	"bytes"
	"encoding/binary"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

const expfExpired = 0x4000

// waitOpenLock waits up to 10s for another process to hold a shared lock on
// bytes 0-3 of hist, the lock a history opener holds. It reports whether it
// saw one (never before the fix).
func waitOpenLock(t *testing.T, hist string) bool {
	t.Helper()
	f, err := os.Open(hist)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	for deadline := time.Now().Add(10 * time.Second); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
		lk := syscall.Flock_t{Type: syscall.F_WRLCK, Whence: 0, Start: 0, Len: 4}
		if syscall.FcntlFlock(f.Fd(), syscall.F_GETLK, &lk) == nil && lk.Type == syscall.F_RDLCK {
			return true
		}
	}
	return false
}

// TestHistoryFastModeRefusedWhileOpen: fast mode (diload -f -u, also
// diloadfromspool -f -u) caches the hash table and writes it back at the
// end, so it must not run on a history another process has open. Every
// opener now holds a shared lock on bytes 0-3, and fast mode asks for an
// exclusive one without waiting. Fails on main: nobody held the lock, and
// diload -f -u ran beside a feeder and a reader.
func TestHistoryFastModeRefusedWhileOpen(t *testing.T) {
	fastLoad := func(t *testing.T, s *Site, hist string) {
		t.Helper()
		saw := waitOpenLock(t, hist)
		out, err := s.TryRun("diload", "-f", "-u", hist)
		if err == nil {
			t.Fatalf("diload -f -u ran on a history that is open elsewhere (open lock seen: %v):\n%s", saw, out)
		}
		if !strings.Contains(out, "in use by another process") {
			t.Errorf("diload -f -u failed without saying the history is in use: %v\n%s", err, out)
		}
	}

	t.Run("feeder", func(t *testing.T) {
		s := NewSite(t, SiteOptions{})
		ids := s.Inject(t, "e2e.test", 1)
		p := s.Peer(t)
		if code, line := p.Cmd("STAT %s", ids[0]); code != 223 {
			t.Fatalf("STAT %s: %d %s", ids[0], code, line)
		}
		fastLoad(t, s, filepath.Join(s.Dir, "dhistory"))
		// the feeder still works
		if code, line := p.Cmd("STAT %s", ids[0]); code != 223 {
			t.Errorf("STAT %s after diload -f -u: %d %s", ids[0], code, line)
		}
	})

	t.Run("reader", func(t *testing.T) {
		s := NewSite(t, SiteOptions{NoStart: true})
		hist := newHistory(t, s)
		_, in, out, done := histCmd(t, s, nil, "dilookup", "-h", hist, "-s")
		fastLoad(t, s, hist)
		in.Close()
		if err := <-done; err != nil {
			t.Errorf("dilookup -s: %v\n%s", err, out.String())
		}
	})
}

// TestHistoryOpenWaitsForFastWriter: the other side of the lock: an opener
// waits for a fast-mode writer to finish instead of reading the hash table
// on disk, which fast mode only writes when it closes. Fails on main:
// dilookup did not wait and did not find the record diload was adding.
func TestHistoryOpenWaitsForFastWriter(t *testing.T) {
	s := NewSite(t, SiteOptions{NoStart: true})
	hist := newHistory(t, s)
	cmd, in, out, done := histCmd(t, s, nil, "diload", "-f", "-u", hist)
	if _, err := io.WriteString(in, dumpLine(0x1234567, 0x89abcde)); err != nil {
		t.Fatal(err)
	}

	// wait until diload holds the fast-mode lock
	f, err := os.Open(hist)
	if err != nil {
		t.Fatal(err)
	}
	held := false
	for deadline := time.Now().Add(30 * time.Second); !held && time.Now().Before(deadline); time.Sleep(5 * time.Millisecond) {
		lk := syscall.Flock_t{Type: syscall.F_RDLCK, Whence: 0, Start: 0, Len: 4}
		held = syscall.FcntlFlock(f.Fd(), syscall.F_GETLK, &lk) == nil &&
			lk.Type == syscall.F_WRLCK && int(lk.Pid) == cmd.Process.Pid
	}
	f.Close()
	if !held {
		t.Fatalf("diload -f never held the history lock\n%s", out.String())
	}

	lcmd, lin, lout, ldone := histCmd(t, s, nil, "dilookup", "-h", hist, "-s")
	if _, err := io.WriteString(lin, "01234567.089abcde\n"); err != nil {
		t.Fatal(err)
	}
	lin.Close()
	if err := waitLockWaiter(lcmd.Process.Pid, ldone); err != nil {
		t.Errorf("dilookup did not wait for the fast-mode writer: %v\n%s", err, lout.String())
	}
	in.Close()
	if err := <-done; err != nil {
		t.Fatalf("diload -f -u: %v\n%s", err, out.String())
	}
	err = <-ldone
	if err != nil || !strings.Contains(lout.String(), "ex=100") {
		t.Errorf("dilookup did not find the record of the fast-mode writer: %v\n%s", err, lout.String())
	}
}

// TestHistoryWriterWaitsForCreator: a writer that finds the history not yet
// created waits for the creator with a shared lock, like a reader. The
// creator now keeps a shared lock while it has the file open, so a wait for
// the exclusive creation lock would last until the creator closes. The test
// is the creator: it holds bytes 0-3, writes a valid history and keeps a
// shared lock; diload must then load its record.
func TestHistoryWriterWaitsForCreator(t *testing.T) {
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

	cmd, in, out, done := histCmd(t, s, nil, "diload", hist)
	io.WriteString(in, dumpLine(0x56789ab, 0xcdef012))
	in.Close()
	if err := waitLockWaiter(cmd.Process.Pid, done); err != nil {
		t.Fatalf("diload did not wait for the history creator: %v\n%s", err, out.String())
	}
	if _, err := f.WriteAt(valid, 0); err != nil {
		t.Fatal(err)
	}
	lk.Type = syscall.F_RDLCK // the creator keeps the history open
	if err := syscall.FcntlFlock(f.Fd(), syscall.F_SETLK, &lk); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("diload: %v\n%s", err, out.String())
		}
	case <-time.After(20 * time.Second):
		t.Fatalf("diload still waits 20s after the creator finished\n%s", out.String())
	}
	if _, found := histExp(t, s, hist, "056789ab.0cdef012"); !found {
		t.Errorf("diload did not load its record\n%s", out.String())
	}
}

// holdHistLock takes a write lock on [start, start+n) of hist in the test
// process; n 0 means the hash chain locks (all of the hash table).
func holdHistLock(t *testing.T, hist string, start, n int64) (release func()) {
	t.Helper()
	f, err := os.OpenFile(hist, os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	if n == 0 {
		var hh [16]byte
		if _, err := f.ReadAt(hh[:], 0); err != nil {
			t.Fatal(err)
		}
		start = int64(binary.NativeEndian.Uint16(hh[12:])) // headSize
		n = int64(binary.NativeEndian.Uint32(hh[4:])) * 4  // hashSize entries
	}
	lk := syscall.Flock_t{Type: syscall.F_WRLCK, Whence: 0, Start: start, Len: n}
	if err := syscall.FcntlFlock(f.Fd(), syscall.F_SETLK, &lk); err != nil {
		t.Fatalf("lock %s at %d: %v", hist, start, err)
	}
	return func() {
		if f != nil {
			f.Close() // releases the lock
			f = nil
		}
	}
}

// TestHistoryAddDuringDhisexpire: dhisexpire copies the history, marks the
// old one dead, copies the records added since and installs the copy. An add
// that saw the old magic before it was marked dead and appends after the last
// copy is lost: its record goes to the old file. The test makes a diload
// wait in that window on a lock it holds: the append lock (bytes 4-7), or
// its hash chain lock. dhisexpire now waits for every chain lock after it
// marks the history dead, and an add re-checks the magic under its chain lock.
// Fails on main: the record is missing from the new history.
func TestHistoryAddDuringDhisexpire(t *testing.T) {
	for _, tc := range []struct {
		name     string
		start, n int64
	}{
		{"append", 4, 4},
		{"chain", 0, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := NewSite(t, SiteOptions{NoStart: true})
			hist := newHistory(t, s)
			release := holdHistLock(t, hist, tc.start, tc.n)
			defer release()

			cmd, in, out, done := histCmd(t, s, nil, "diload", hist)
			if _, err := io.WriteString(in, dumpLine(0x2345678, 0x9abcdef)); err != nil {
				t.Fatal(err)
			}
			if err := waitLockWaiter(cmd.Process.Pid, done); err != nil {
				t.Fatalf("diload did not wait for the test's lock: %v\n%s", err, out.String())
			}

			xcmd, xin, xout, xdone := histCmd(t, s, nil, "dhisexpire", "-a", hist)
			xin.Close()
			// main finishes at once; the fix waits for diload's chain lock
			_ = waitLockWaiter(xcmd.Process.Pid, xdone)

			release()
			in.Close()
			if err := <-done; err != nil {
				t.Fatalf("diload: %v\n%s", err, out.String())
			}
			if err := <-xdone; err != nil {
				t.Fatalf("dhisexpire -a: %v\n%s", err, xout.String())
			}
			if _, found := histExp(t, s, hist, "02345678.09abcdef"); !found {
				t.Errorf("the record added during dhisexpire is not in the new history\ndiload:\n%s\ndhisexpire:\n%s",
					out.String(), xout.String())
			}
		})
	}
}

// TestDcancelAfterHistorySwap: dcancel -h expires records with HistoryExpire,
// which did not check the magic. After dhisexpire -a installed a new history,
// a running dcancel kept writing to the old file, so every later cancel was
// lost. Fails on main: the second article is not expired in the new history.
func TestDcancelAfterHistorySwap(t *testing.T) {
	s := NewSite(t, SiteOptions{})
	ids := s.Inject(t, "e2e.test", 2)
	hist := filepath.Join(s.Dir, "dhistory")

	_, in, out, done := histCmd(t, s, nil, "dcancel", "-h")
	io.WriteString(in, ids[0]+"\n")
	waitFor(t, 30*time.Second, "the first cancel", func() bool {
		exp, _ := histExp(t, s, hist, ids[0])
		return exp&expfExpired != 0
	})
	s.Run(t, "dhisexpire", "-a", hist)
	io.WriteString(in, ids[1]+"\n")
	in.Close()
	if err := <-done; err != nil {
		t.Fatalf("dcancel -h: %v\n%s", err, out.String())
	}
	for i, id := range ids {
		if exp, found := histExp(t, s, hist, id); !found || exp&expfExpired == 0 {
			t.Errorf("article %d %s after dcancel -h: found %v, exp %#x, want expired\n%s",
				i, id, found, exp, out.String())
		}
	}
}

// renameHoldSource is an LD_PRELOAD shim: with RENAMEHOLD=dir, rename()
// creates dir/reached and waits for dir/go before it renames.
const renameHoldSource = `#define _GNU_SOURCE
#include <dlfcn.h>
#include <fcntl.h>
#include <stdio.h>
#include <stdlib.h>
#include <unistd.h>

int
rename(const char *from, const char *to)
{
    int (*real)(const char *, const char *) =
	(int (*)(const char *, const char *))dlsym(RTLD_NEXT, "rename");
    const char *dir = getenv("RENAMEHOLD");
    char p[4096];
    int i, fd;

    if (dir != NULL) {
	snprintf(p, sizeof(p), "%s/reached", dir);
	if ((fd = open(p, O_WRONLY|O_CREAT, 0644)) >= 0)
	    close(fd);
	snprintf(p, sizeof(p), "%s/go", dir);
	for (i = 0; i < 6000 && access(p, F_OK) != 0; i++)
	    usleep(10000);
    }
    return real(from, to);
}
`

// TestDcancelRewriteStoresLatestRecord: dcancel -s rewrites a spool file,
// renames it into place and then stores each article's new offset with
// HistoryStore, a copy of the record it looked up before the rewrite. The
// shim stops dcancel at that rename while the test changes the history:
//
//	expired  dcancel -h expires a copied article; the late store revived it
//	swapped  dhisexpire -a installs a new history; the store went to the old
//	         file, and the new one kept offsets into the old spool file
//
// HistoryStore now works under the chain lock, on the current history, and
// keeps an expired flag it finds. Both fail on main.
func TestDcancelRewriteStoresLatestRecord(t *testing.T) {
	so := preloadShim(t, "renamehold", renameHoldSource)
	for _, mode := range []string{"expired", "swapped"} {
		t.Run(mode, func(t *testing.T) {
			s := NewSite(t, SiteOptions{})
			ids := s.Inject(t, "e2e.test", 3)
			hist := filepath.Join(s.Dir, "dhistory")
			hold := t.TempDir()

			_, in, out, done := histCmd(t, s, []string{lockFaultEnv(so), "RENAMEHOLD=" + hold}, "dcancel", "-s")
			io.WriteString(in, ids[0]+"\n")
			in.Close()
			waitFor(t, 30*time.Second, "dcancel -s at its rename", func() bool {
				return fileExists(filepath.Join(hold, "reached"))
			})
			if mode == "expired" {
				dcancelRun(t, s, ids[1]+"\n", "-h")
			} else {
				s.Run(t, "dhisexpire", "-a", hist)
			}
			if err := os.WriteFile(filepath.Join(hold, "go"), nil, 0o644); err != nil {
				t.Fatal(err)
			}
			if err := <-done; err != nil {
				t.Fatalf("dcancel -s: %v\n%s", err, out.String())
			}

			p := s.Peer(t)
			for i, id := range ids[1:] {
				exp, found := histExp(t, s, hist, id)
				if !found {
					t.Errorf("article %d %s is not in the history", i+1, id)
					continue
				}
				if mode == "expired" && i == 0 {
					if exp&expfExpired == 0 {
						t.Errorf("article 1 %s was expired during the rewrite and is live again (exp %#x)\n%s",
							id, exp, out.String())
					}
					continue
				}
				code, line := p.Cmd("ARTICLE %s", id)
				if code != 220 {
					t.Errorf("ARTICLE %d %s after the rewrite: %d %s\n%s", i+1, id, code, line, out.String())
					continue
				}
				if body := strings.Join(p.ReadDotLines(), "\n"); !strings.Contains(body, id) {
					t.Errorf("ARTICLE %d %s after the rewrite returned a different article:\n%s", i+1, id, body)
				}
			}
		})
	}
}

// cpuTicks is the user+system CPU time of process pid in clock ticks (Linux
// /proc), or -1.
func cpuTicks(pid int) int {
	b, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
	if err != nil {
		return -1
	}
	s := string(b)
	f := strings.Fields(s[strings.LastIndex(s, ")")+2:])
	u, _ := strconv.Atoi(f[11])
	k, _ := strconv.Atoi(f[12])
	return u + k
}

// setMagic writes magic into the header of the history open as f.
func setMagic(t *testing.T, f *os.File, magic uint32) {
	t.Helper()
	var m [4]byte
	binary.NativeEndian.PutUint32(m[:], magic)
	if _, err := f.WriteAt(m[:], 0); err != nil {
		t.Fatal(err)
	}
}

// TestHistoryFirstOpenOfDeadHistory: a program that starts while dhisexpire
// has the history marked dead (dhisexpire, an idle feeder or dlockhistory
// still holding the open lock; the test also holds dhisexpire's marker lock
// on bytes 8-11) waits for the new history by name, as a reopen does,
// without using the CPU. A history with data and a wrong magic
// is refused at once. The test holds the shared open lock. Fails on main
// (dilookup exits at once on a dead history, writers wait for the lock and
// never see the history come back) and on e3f746a (writers spin).
func TestHistoryFirstOpenOfDeadHistory(t *testing.T) {
	const hmagic, hdead, bad = 0xA1B2C3D4, 0xDEADF5E6, 0x12345678
	for _, tool := range []string{"diload", "dhisctl", "dilookup"} {
		for _, magic := range []uint32{hdead, bad} {
			name := tool + "/dead"
			if magic == bad {
				name = tool + "/corrupt"
			}
			t.Run(name, func(t *testing.T) {
				s := NewSite(t, SiteOptions{NoStart: true})
				hist := newHistory(t, s)
				f, err := os.OpenFile(hist, os.O_RDWR, 0)
				if err != nil {
					t.Fatal(err)
				}
				defer f.Close()
				setMagic(t, f, magic)
				lk := syscall.Flock_t{Type: syscall.F_RDLCK, Start: 0, Len: 4}
				if err := syscall.FcntlFlock(f.Fd(), syscall.F_SETLK, &lk); err != nil {
					t.Fatal(err)
				}
				// the lock a running dhisexpire holds while the history is dead
				mk := syscall.Flock_t{Type: syscall.F_WRLCK, Start: 8, Len: 4}
				if err := syscall.FcntlFlock(f.Fd(), syscall.F_SETLK, &mk); err != nil {
					t.Fatal(err)
				}
				args := map[string][]string{
					"diload":   {hist},
					"dhisctl":  {"-e", hist, "<a@e2e.test>"},
					"dilookup": {"-h", hist, "<a@e2e.test>"},
				}[tool]
				cmd, in, out, done := histCmd(t, s, nil, tool, args...)
				in.Close()
				if magic == bad {
					select {
					case err := <-done:
						if err == nil || !strings.Contains(out.String(), "corrupted history file") {
							t.Errorf("%s on a corrupt history:\n%s", tool, out.String())
						}
					case <-time.After(10 * time.Second):
						t.Errorf("%s still runs 10s after it opened a corrupt history (cpu ticks %d)",
							tool, cpuTicks(cmd.Process.Pid))
					}
					return
				}
				select {
				case err := <-done:
					t.Fatalf("%s did not wait for the new history: %v\n%s", tool, err, out.String())
				case <-time.After(3 * time.Second):
				}
				if n := cpuTicks(cmd.Process.Pid); n > 50 {
					t.Errorf("%s used %d cpu ticks in 3s while it waited", tool, n)
				}
				setMagic(t, f, hmagic) // as dhisexpire -u, or the new file
				select {
				case <-done:
					if o := out.String(); strings.Contains(o, "corrupted") {
						t.Errorf("%s did not use the history once it was valid again:\n%s", tool, o)
					}
				case <-time.After(15 * time.Second):
					t.Errorf("%s still waits 15s after the history is valid again\n%s", tool, out.String())
				}
			})
		}
	}
}

// pwriteHoldSource is an LD_PRELOAD shim: with PWHOLD=dir, the first 2-byte
// pwrite (an exp update) creates dir/reached and waits for dir/go, before
// the write, or after it with PWAFTER set.
const pwriteHoldSource = `#define _GNU_SOURCE
#include <dlfcn.h>
#include <fcntl.h>
#include <stdio.h>
#include <stdlib.h>
#include <unistd.h>
#include <sys/types.h>

static void
hold(size_t n)
{
    const char *dir = getenv("PWHOLD");
    char p[4096];
    int i, fd;
    static int done;

    if (dir == NULL || n != 2 || done++)
	return;
    snprintf(p, sizeof(p), "%s/reached", dir);
    if ((fd = open(p, O_WRONLY|O_CREAT, 0644)) >= 0)
	close(fd);
    snprintf(p, sizeof(p), "%s/go", dir);
    for (i = 0; i < 6000 && access(p, F_OK) != 0; i++)
	usleep(10000);
}

#ifndef __sun	/* 64-bit illumos names pwrite64 pwrite */
ssize_t
pwrite64(int fd, const void *b, size_t n, off_t o)
{
    ssize_t (*real)(int, const void *, size_t, off_t) =
	(ssize_t (*)(int, const void *, size_t, off_t))dlsym(RTLD_NEXT, "pwrite64");
    ssize_t r;

    if (getenv("PWAFTER") == NULL)
	hold(n);
    r = real(fd, b, n, o);
    if (getenv("PWAFTER") != NULL)
	hold(n);
    return r;
}
#endif

ssize_t
pwrite(int fd, const void *b, size_t n, off_t o)
{
    ssize_t (*real)(int, const void *, size_t, off_t) =
	(ssize_t (*)(int, const void *, size_t, off_t))dlsym(RTLD_NEXT, "pwrite");
    ssize_t r;

    if (getenv("PWAFTER") == NULL)
	hold(n);
    r = real(fd, b, n, o);
    if (getenv("PWAFTER") != NULL)
	hold(n);
    return r;
}
`

// TestHistoryUpdateDuringDhisexpire: dcancel -h is in HistoryExpire, under
// the chain lock, when dhisexpire -a marks the history dead. dhisexpire waits
// for the lock, but it copied the record before: the update sees the dead
// magic after its write and does it again in the new history.
//
//	expire    stopped before its write; fails on main and on e3f746a:
//	          dcancel reports the cancel and the new history lacks it
//	unexpire  dcancel -h -u stopped after its write; the redo must not
//	          expire again a record that the copy has without the flag
//	          (it did before an unexpire left live records alone)
func TestHistoryUpdateDuringDhisexpire(t *testing.T) {
	so := preloadShim(t, "pwhold", pwriteHoldSource)
	for _, unexp := range []bool{false, true} {
		name := "expire"
		if unexp {
			name = "unexpire"
		}
		t.Run(name, func(t *testing.T) {
			s := NewSite(t, SiteOptions{})
			ids := s.Inject(t, "e2e.test", 1)
			hist := filepath.Join(s.Dir, "dhistory")
			hold := t.TempDir()
			env := []string{lockFaultEnv(so), "PWHOLD=" + hold}
			args := []string{"-h"}
			if unexp {
				dcancelRun(t, s, ids[0]+"\n", "-h")
				env = append(env, "PWAFTER=1")
				args = []string{"-h", "-u"}
			}
			_, in, out, done := histCmd(t, s, env, "dcancel", args...)
			io.WriteString(in, ids[0]+"\n")
			in.Close()
			waitFor(t, 30*time.Second, "dcancel in its update", func() bool {
				return fileExists(filepath.Join(hold, "reached"))
			})
			xcmd, xin, xout, xdone := histCmd(t, s, nil, "dhisexpire", "-a", hist)
			xin.Close()
			_ = waitLockWaiter(xcmd.Process.Pid, xdone) // main does not wait
			if err := os.WriteFile(filepath.Join(hold, "go"), nil, 0o644); err != nil {
				t.Fatal(err)
			}
			if err := <-done; err != nil {
				t.Fatalf("dcancel %v: %v\n%s", args, err, out.String())
			}
			if err := <-xdone; err != nil {
				t.Fatalf("dhisexpire -a: %v\n%s", err, xout.String())
			}
			exp, found := histExp(t, s, hist, ids[0])
			if !found || (exp&expfExpired != 0) == unexp {
				t.Errorf("dcancel %v in progress during dhisexpire: found %v exp %#x in the new history\n%s",
					args, found, exp, out.String())
			}
		})
	}
}

// TestDcancelUnexpireLiveRecord: dcancel -h -u leaves a record that is not
// expired alone. Fails on main: it expired it.
func TestDcancelUnexpireLiveRecord(t *testing.T) {
	s := NewSite(t, SiteOptions{})
	ids := s.Inject(t, "e2e.test", 1)
	out := dcancelRun(t, s, ids[0]+"\n", "-h", "-u")
	if exp, _ := histExp(t, s, filepath.Join(s.Dir, "dhistory"), ids[0]); exp&expfExpired != 0 {
		t.Errorf("dcancel -h -u expired a live record (exp %#x)\n%s", exp, out)
	}
}

// TestHistoryDeadWithoutDhisexpire: a history left marked dead (dhisexpire
// without -a, or one that was killed) is not replaced by anyone: with no
// dhisexpire holding its marker lock, a first open and a reopen fail at once
// and say what to do, instead of waiting six minutes. Fails on 111d730 (both
// waited) and on main (the open failed without saying why, the reopen
// retried an empty name).
func TestHistoryDeadWithoutDhisexpire(t *testing.T) {
	for _, mode := range []string{"open", "reopen"} {
		t.Run(mode, func(t *testing.T) {
			s := NewSite(t, SiteOptions{NoStart: true})
			hist := newHistory(t, s)
			var in io.WriteCloser
			var out *bytes.Buffer
			var done chan error
			if mode == "reopen" {
				_, in, out, done = histCmd(t, s, nil, "diload", hist)
				io.WriteString(in, dumpLine(0x6789abc, 0xdef0123))
				waitFor(t, 30*time.Second, "the first add", func() bool {
					_, found := histExp(t, s, hist, "06789abc.0def0123")
					return found
				})
			}
			f, err := os.OpenFile(hist, os.O_RDWR, 0)
			if err != nil {
				t.Fatal(err)
			}
			setMagic(t, f, 0xDEADF5E6)
			f.Close()
			if mode == "open" {
				_, in, out, done = histCmd(t, s, nil, "dilookup", "-h", hist, "<a@e2e.test>")
			} else {
				io.WriteString(in, dumpLine(0x789abcd, 0xef01234))
			}
			in.Close()
			select {
			case err := <-done:
				if err == nil || !strings.Contains(out.String(), "no dhisexpire is running") {
					t.Errorf("%s of a history left dead: %v\n%s", mode, err, out.String())
				}
			case <-time.After(15 * time.Second):
				t.Errorf("%s of a history left dead still waits after 15s\n%s", mode, out.String())
			}
		})
	}
}

// TestHistoryFirstOpenDuringDhisexpire: a program that starts while a real
// dhisexpire -a has the history marked dead (here it waits for the test,
// which holds every chain lock) waits, then uses the new history: the
// marker lock keeps the wait where one is due. Fails on main (no wait).
func TestHistoryFirstOpenDuringDhisexpire(t *testing.T) {
	s := NewSite(t, SiteOptions{NoStart: true})
	hist := newHistory(t, s)
	l := exec.Command(filepath.Join(s.Bin, "diload"), hist)
	l.Dir, l.Env = s.Dir, append(os.Environ(), s.Env...)
	l.Stdin = strings.NewReader(dumpLine(0x89abcde, 0xf012345))
	if b, err := l.CombinedOutput(); err != nil {
		t.Fatalf("diload: %v\n%s", err, b)
	}
	release := holdHistLock(t, hist, 0, 0)
	defer release()
	xcmd, xin, xout, xdone := histCmd(t, s, nil, "dhisexpire", "-a", hist)
	xin.Close()
	if err := waitLockWaiter(xcmd.Process.Pid, xdone); err != nil {
		t.Fatalf("dhisexpire did not wait for the test's chain locks: %v\n%s", err, xout.String())
	}
	_, in, out, done := histCmd(t, s, nil, "dilookup", "-h", hist, "089abcde.0f012345")
	in.Close()
	select {
	case err := <-done:
		t.Fatalf("dilookup did not wait while dhisexpire ran: %v\n%s", err, out.String())
	case <-time.After(2 * time.Second):
	}
	release()
	if err := <-xdone; err != nil {
		t.Fatalf("dhisexpire -a: %v\n%s", err, xout.String())
	}
	select {
	case err := <-done:
		if err != nil || !strings.Contains(out.String(), "ex=100") {
			t.Errorf("dilookup after dhisexpire: %v\n%s", err, out.String())
		}
	case <-time.After(15 * time.Second):
		t.Errorf("dilookup still waits 15s after dhisexpire finished\n%s", out.String())
	}
}

// TestHistoryUpdateLockFailure: a chain lock that keeps failing (ENOLCK
// from the fault shim, after the retries) is an error, not "no history
// entry" or success. Fails on main and on e3f746a: dcancel -h and dhisctl
// -e exit 0 and the record is not expired.
func TestHistoryUpdateLockFailure(t *testing.T) {
	so := lockFaultShim(t)
	s := NewSite(t, SiteOptions{})
	ids := s.Inject(t, "e2e.test", 1)
	hist := filepath.Join(s.Dir, "dhistory")
	env := []string{lockFaultEnv(so), "LOCKFAULT=enolck", "LOCKFAULT_FILE=dhistory"}
	for _, c := range []struct {
		name string
		args []string
	}{
		{"dcancel", []string{"-h"}},
		{"dhisctl", []string{"-e", hist, ids[0]}},
	} {
		_, in, out, done := histCmd(t, s, env, c.name, c.args...)
		io.WriteString(in, ids[0]+"\n")
		in.Close()
		err := <-done
		if err == nil || !strings.Contains(out.String(), "Unable to update the history entry") {
			t.Errorf("%s with a failing chain lock: %v\n%s", c.name, err, out.String())
		}
	}
	if exp, _ := histExp(t, s, hist, ids[0]); exp&expfExpired != 0 {
		t.Errorf("the record was expired without its lock (exp %#x)", exp)
	}
}

// TestDhisexpireWriterTimeout: dhisexpire waits for running writers after it
// marks the history dead, but at most -w seconds: a writer that hangs (here
// the test, holding every chain lock) must not keep the history dead. It
// gives up, marks the history valid again and removes the new one. Fails on
// main, which has no -w.
func TestDhisexpireWriterTimeout(t *testing.T) {
	s := NewSite(t, SiteOptions{NoStart: true})
	hist := newHistory(t, s)
	release := holdHistLock(t, hist, 0, 0)
	defer release()
	_, in, out, done := histCmd(t, s, nil, "dhisexpire", "-w", "1", "-a", hist)
	in.Close()
	select {
	case err := <-done:
		if err == nil || !strings.Contains(out.String(), "Timed out waiting for running history writers") {
			t.Errorf("dhisexpire with a hung writer: %v\n%s", err, out.String())
		}
	case <-time.After(30 * time.Second):
		t.Fatalf("dhisexpire still waits 30s for a hung writer\n%s", out.String())
	}
	release()
	if fileExists(hist + ".new") {
		t.Errorf("dhisexpire left %s.new", hist)
	}
	// valid again: a reader opens it at once
	if o, err := s.TryRun("dilookup", "-h", hist, "<a@e2e.test>"); !strings.Contains(o, "Not Found") {
		t.Errorf("the history is not valid after the timeout: %v\n%s", err, o)
	}
}

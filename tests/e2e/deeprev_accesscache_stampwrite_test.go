package e2e

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	testkit "github.com/go-while/nntp-testkit"
)

// stampWriteFailSource makes write() to a file whose name ends in "/.accesslock" fail with
// ENOSPC while the file STAMPFAIL_FLAG exists, and marks each refused write with the time of
// CLOCK_MONOTONIC in seconds ("refused write to <path> at <s>.<ms>", refusedWriteTimes()).
const stampWriteFailSource = `#define _GNU_SOURCE
#define __EXTENSIONS__
#include <dlfcn.h>
#include <errno.h>
#include <fcntl.h>
#include <stdio.h>
#include <string.h>
#include <sys/types.h>
#include <time.h>
#include <unistd.h>
/*FDPATH*/
/*MARK*/
ssize_t
write(int fd, const void *buf, size_t n)
{
    static ssize_t (*real)(int, const void *, size_t);
    const char *flag = getenv("STAMPFAIL_FLAG");

    if (real == NULL)
	real = (ssize_t (*)(int, const void *, size_t))dlsym(RTLD_NEXT, "write");
    if (flag != NULL && access(flag, F_OK) == 0) {
	char path[4096];
	size_t l;

	fdpath(fd, path, sizeof(path));
	l = strlen(path);
	if (l >= 12 && strcmp(path + l - 12, "/.accesslock") == 0) {
	    struct timespec ts;

	    clock_gettime(CLOCK_MONOTONIC, &ts);
	    shim_mark("refused write to %s at %ld.%03ld", path, (long)ts.tv_sec, (long)(ts.tv_nsec / 1000000));
	    errno = ENOSPC;
	    return -1;
	}
    }
    return real(fd, buf, n);
}
`

// refusedWriteTimes returns the times (seconds of CLOCK_MONOTONIC, as the injector took them)
// of the stamp writes stampWriteFailSource refused, in the order of the mark file, and the
// marks of refused writes without a time it could read.
func refusedWriteTimes(mark string) (at []float64, untimed []string) {
	for _, l := range shimMarked(mark, "refused write") {
		i := strings.LastIndex(l, " at ")
		v, err := 0.0, errors.New("no time")
		if i >= 0 {
			v, err = strconv.ParseFloat(l[i+4:], 64)
		}
		if err != nil {
			untimed = append(untimed, l)
			continue
		}
		at = append(at, v)
	}
	return at, untimed
}

// retryGaps returns the intervals in seconds between the times at, each rounded to the
// millisecond, for the log: the first 12, then how many more there are.
func retryGaps(at []float64) []string {
	var g []string
	for i := 1; i < len(at); i++ {
		if len(g) == 12 {
			g = append(g, "and "+strconv.Itoa(len(at)-i)+" more")
			break
		}
		g = append(g, strconv.FormatFloat(at[i]-at[i-1], 'f', 3, 64))
	}
	return g
}

// TestDeepRevAccessCacheStampWriteFails: the install of the access cache announces the new
// set by writing the lock file's stamp. That write was not checked: when it failed (a full
// disk) the install counted as done and was never tried again, while the lock file stayed
// empty and the readers (rightly) loaded nothing, so every client got 502 until dreader.access
// was changed or dreaderd restarted.
//
// A fresh site starts while a preloaded write() refuses writes to .accesslock (ENOSPC): the
// client must get the 502 refusal and the lock file must stay empty. Once the fault is gone,
// the next try must install it: a client is served within 10 s.
func TestDeepRevAccessCacheStampWriteFails(t *testing.T) {
	so := preloadShim(t, "stampfail", withMark(withFdPath(stampWriteFailSource)))
	dir := t.TempDir()
	flag := filepath.Join(dir, "fail")
	mark := filepath.Join(dir, "mark")
	if err := os.WriteFile(flag, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	_, spool := spoolStore(t)
	s := NewReaderSite(t, ReaderOptions{Spool: spool, NoStart: true})
	s.Env = append(s.Env, lockFaultEnv(so), "SHIM_MARK="+mark, "STAMPFAIL_FLAG="+flag)
	t.Cleanup(func() {
		if t.Failed() {
			t.Logf("general.log:\n%s\nmarks:\n%s", generalLog(s), strings.Join(shimMarks(mark), "\n"))
		}
	})
	startReaderArgs(t, s)

	var g string
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if g = readerGreeting(s.Addr); g != "" && len(shimMarked(mark, "refused write")) > 0 {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	if len(shimMarked(mark, "refused write")) == 0 {
		t.Fatalf("the stamp of the lock file was not written through the injector")
	}
	if !strings.HasPrefix(g, "502 ") {
		t.Fatalf("with the stamp unwritten the client got %q, want the 502 refusal (no access cache loaded)", strings.TrimSpace(g))
	}
	if fi, err := os.Stat(filepath.Join(s.Dir, ".accesslock")); err != nil || fi.Size() != 0 {
		t.Fatalf("during the fault the lock file must exist and be empty (no stamp): %v %v", fi, err)
	}

	if err := os.Remove(flag); err != nil {
		t.Fatal(err)
	}
	deadline = time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if g = readerGreeting(s.Addr); strings.HasPrefix(g, "20") {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	if !strings.HasPrefix(g, "20") {
		t.Errorf("10 s after the stamp could be written again the client still gets %q: the install was not tried again (%d times \"Installing access cache\")",
			strings.TrimSpace(g), strings.Count(generalLog(s), "Installing access cache"))
	}
	if fi, err := os.Stat(filepath.Join(s.Dir, ".accesslock")); err != nil {
		t.Errorf("the lock file after the fault: %v", err)
	} else if fi.Size() == 0 {
		t.Errorf("the lock file has no stamp (0 bytes) after the fault")
	}
}

// TestDeepRevAccessCacheStampWriteFailsLater: the stamp write fails on a later install, when
// the lock file has the stamp of an earlier one. The running processes do not see a new
// stamp, so they keep the set they have; the install must be retried, and the lock file must
// keep its old stamp (size and time) while the fault lasts. What this test asserts: every
// LIST ACTIVE during the fault shows one whole set, A's [e2e.test] or B's [e2e.other], never
// both groups; the install is repeated; the stamp is unchanged; once the fault is gone, B is
// in use. (Both sets use the readerdef name rp; it does not check every kind of change.)
func TestDeepRevAccessCacheStampWriteFailsLater(t *testing.T) {
	so := preloadShim(t, "stampfail", withMark(withFdPath(stampWriteFailSource)))
	dir := t.TempDir()
	flag := filepath.Join(dir, "fail")
	mark := filepath.Join(dir, "mark")
	_, spool := spoolStore(t)
	s := NewReaderSite(t, ReaderOptions{Spool: spool, Access: accessSetA, NoStart: true})
	s.Env = append(s.Env, lockFaultEnv(so), "SHIM_MARK="+mark, "STAMPFAIL_FLAG="+flag)
	t.Cleanup(func() {
		if t.Failed() {
			t.Logf("general.log:\n%s\nmarks:\n%s", generalLog(s), strings.Join(shimMarks(mark), "\n"))
		}
	})
	startReader(t, s, "[127.0.0.1]:"+strings.TrimPrefix(s.Addr, "127.0.0.1:"))
	list := func() []string {
		t.Helper()
		c := s.Reader(t)
		defer c.Close()
		c.ExpectCmd(215, "LIST ACTIVE")
		names := activeNames(c.ReadDotLines())
		c.ExpectCmd(205, "QUIT")
		return names
	}
	if got := list(); !slices.Equal(got, []string{"e2e.test"}) {
		t.Fatalf("with dreader.access A: groups %q, want [e2e.test]", got)
	}
	lock := filepath.Join(s.Dir, ".accesslock")
	fi0, err := os.Stat(lock)
	if err != nil || fi0.Size() == 0 {
		t.Fatalf("the lock file has no stamp after the first install: %v %v", fi0, err)
	}

	if err := os.WriteFile(flag, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	s.WriteFile(t, "dreader.access", accessSetB)
	testkit.WaitFor(t, 20*time.Second, "a refused stamp write of the later install", func() bool {
		return len(shimMarked(mark, "refused write")) > 0
	})
	m0 := len(shimMarked(mark, "refused write"))
	// a failed install is tried again after 5 s (ACCESS_RETRY_MS): look for 8
	for i := 0; i < 8; i++ {
		got := list()
		if !slices.Equal(got, []string{"e2e.test"}) && !slices.Equal(got, []string{"e2e.other"}) {
			t.Errorf("during the fault: groups %q, want one whole set (A's [e2e.test] or B's [e2e.other])", got)
		}
		time.Sleep(time.Second)
	}
	if m := len(shimMarked(mark, "refused write")); m <= m0 {
		t.Errorf("the install was not tried again during 8 s of the fault (no further refused stamp write)")
	}
	if fi1, err := os.Stat(lock); err != nil || fi1.Size() != fi0.Size() || !fi1.ModTime().Equal(fi0.ModTime()) {
		t.Errorf("the lock file's stamp changed during the fault: before %v, now %v %v", fi0.ModTime(), fi1, err)
	}
	waitB(t, flag, list)
}

// TestDeepRevAccessCacheRetryPaced: InstallAccessCache() runs on every pass of the main
// loop, and a pass comes with every client. A failed install was redone on the next pass, so
// while the cause lasted (a full disk) the main process read dreader.access, wrote the five
// cache files, renamed them and logged a LOG_CRIT line for every client: about 2600 installs
// in 4 s, and it accepted about 7 times fewer connections. A failed install is now tried
// again after 5 s (ACCESS_RETRY_MS).
//
// While a preloaded write() refuses the stamp of the lock file (ENOSPC), dreader.access A is
// replaced by B and clients connect without a pause for 11 s. The injector notes the time of
// each refused stamp write (one per install): no two may be less than 4.5 s apart, and at
// least one retry must come in the 11 s. AccessInstallFailed() sets the 5 s wait right after
// the refused write, and the next try writes its stamp only after it has built the five
// files, so a late or slow pass makes an interval longer, never shorter; the 0.5 s below 5 s
// leave room for the few calls between the refused write and AccessInstallFailed(). Every
// install must reach the stamp: no more "Installing access cache" lines than refused writes
// (plus one under way). Once the fault is gone, B must be installed.
func TestDeepRevAccessCacheRetryPaced(t *testing.T) {
	so := preloadShim(t, "stampfail", withMark(withFdPath(stampWriteFailSource)))
	dir := t.TempDir()
	flag := filepath.Join(dir, "fail")
	mark := filepath.Join(dir, "mark")
	_, spool := spoolStore(t)
	s := NewReaderSite(t, ReaderOptions{Spool: spool, Access: accessSetA, NoStart: true})
	s.Env = append(s.Env, lockFaultEnv(so), "SHIM_MARK="+mark, "STAMPFAIL_FLAG="+flag)
	t.Cleanup(func() {
		if t.Failed() {
			log := generalLog(s)
			if len(log) > 20000 {
				log = "...\n" + log[len(log)-20000:]
			}
			t.Logf("general.log:\n%s", log)
		}
	})
	startReader(t, s, "[127.0.0.1]:"+strings.TrimPrefix(s.Addr, "127.0.0.1:"))
	list := func() []string {
		t.Helper()
		c := s.Reader(t)
		defer c.Close()
		c.ExpectCmd(215, "LIST ACTIVE")
		names := activeNames(c.ReadDotLines())
		c.ExpectCmd(205, "QUIT")
		return names
	}
	if got := list(); !slices.Equal(got, []string{"e2e.test"}) {
		t.Fatalf("with dreader.access A: groups %q, want [e2e.test]", got)
	}

	if err := os.WriteFile(flag, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	s.WriteFile(t, "dreader.access", accessSetB)
	testkit.WaitFor(t, 20*time.Second, "a refused stamp write of the install of B", func() bool {
		return len(shimMarked(mark, "refused write")) > 0
	})
	installs := func() int { return strings.Count(generalLog(s), "Installing access cache") }
	at, _ := refusedWriteTimes(mark)
	i0, m0 := installs(), len(at)
	n := 0
	for start := time.Now(); time.Since(start) < 11*time.Second; n++ {
		readerGreeting(s.Addr)
	}
	at, untimed := refusedWriteTimes(mark)
	i1 := installs()
	t.Logf("%d connections in 11 s of the fault: %d installs, %d refused stamp writes, intervals %v s",
		n, i1-i0, len(at)-m0, retryGaps(at))
	if n < 20 {
		t.Fatalf("only %d connections in 11 s: too few passes of the main loop for the test to mean anything", n)
	}
	if len(untimed) > 0 {
		t.Fatalf("refused stamp writes without their time: %q", untimed)
	}
	short, least := 0, 0.0
	for i := 1; i < len(at); i++ {
		if d := at[i] - at[i-1]; d < 4.5 {
			if short == 0 || d < least {
				least = d
			}
			short++
		}
	}
	if short > 0 {
		t.Errorf("the failed install is tried again too early: %d of %d intervals between refused stamp writes under %.1f s, the shortest %.3f s (want 5 s)",
			short, len(at)-1, 4.5, least)
	}
	if len(at) == m0 {
		t.Errorf("the failed install was not tried again in 11 s of the fault with %d connections (want a try every 5 s)", n)
	}
	if i1-i0 > len(at)-m0+1 {
		t.Errorf("%d installs but %d refused stamp writes in 11 s: installs that did not reach the stamp", i1-i0, len(at)-m0)
	}
	waitB(t, flag, list)
}

// closeFailSource makes close() of the descriptor of .accesslock that was opened for writing
// close it and then report EIO, while the file CLOSEFAIL_FLAG exists; each one is marked.
const closeFailSource = `#define _GNU_SOURCE
#define __EXTENSIONS__
#include <dlfcn.h>
#include <errno.h>
#include <fcntl.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/types.h>
#include <unistd.h>
/*FDPATH*/
/*MARK*/
int
close(int fd)
{
    static int (*real)(int);
    const char *flag = getenv("CLOSEFAIL_FLAG");
    int bad = 0;
    int r;

    if (real == NULL)
	real = (int (*)(int))dlsym(RTLD_NEXT, "close");
    if (flag != NULL && access(flag, F_OK) == 0) {
	char path[4096];
	size_t l;

	fdpath(fd, path, sizeof(path));
	l = strlen(path);
	if (l >= 12 && strcmp(path + l - 12, "/.accesslock") == 0 &&
	    (fcntl(fd, F_GETFL) & O_ACCMODE) == O_RDWR)
	    bad = 1;
    }
    r = real(fd);
    if (bad && r == 0) {
	shim_mark("failed close of .accesslock");
	errno = EIO;
	return -1;
    }
    return r;
}
`

// TestDeepRevAccessCacheStampCloseFails: when close() of the lock file fails after its stamp
// was written, the set is announced (the stamp is there), so the log must not say "not
// announced" (as it did); it says that the close failed, and the install is tried again,
// as a failed close may hide a write that did not reach the disk.
//
// While a preloaded close() reports EIO for .accesslock, dreader.access A is replaced by B:
// the log names the failed close and never "not announced", the clients get B, the install
// is retried while the fault lasts and no more once it is gone.
func TestDeepRevAccessCacheStampCloseFails(t *testing.T) {
	if runtime.GOOS == "freebsd" || runtime.GOOS == "netbsd" {
		t.Skip("the close() injector cannot go with the descriptor-name tracker these systems need (it defines close() too)")
	}
	so := preloadShim(t, "closefail", withMark(withFdPath(closeFailSource)))
	dir := t.TempDir()
	flag := filepath.Join(dir, "fail")
	mark := filepath.Join(dir, "mark")
	_, spool := spoolStore(t)
	s := NewReaderSite(t, ReaderOptions{Spool: spool, Access: accessSetA, NoStart: true})
	s.Env = append(s.Env, lockFaultEnv(so), "SHIM_MARK="+mark, "CLOSEFAIL_FLAG="+flag)
	t.Cleanup(func() {
		if t.Failed() {
			t.Logf("general.log:\n%s\nmarks:\n%s", generalLog(s), strings.Join(shimMarks(mark), "\n"))
		}
	})
	startReader(t, s, "[127.0.0.1]:"+strings.TrimPrefix(s.Addr, "127.0.0.1:"))
	list := func() []string {
		t.Helper()
		c := s.Reader(t)
		defer c.Close()
		c.ExpectCmd(215, "LIST ACTIVE")
		names := activeNames(c.ReadDotLines())
		c.ExpectCmd(205, "QUIT")
		return names
	}
	if got := list(); !slices.Equal(got, []string{"e2e.test"}) {
		t.Fatalf("with dreader.access A: groups %q, want [e2e.test]", got)
	}
	installs := func() int { return strings.Count(generalLog(s), "Installing access cache") }

	if err := os.WriteFile(flag, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	s.WriteFile(t, "dreader.access", accessSetB)
	testkit.WaitFor(t, 15*time.Second, "a failed close of the lock file", func() bool {
		return len(shimMarked(mark, "failed close")) > 0
	})
	// each try ends in a failed close; the main process tries on its next pass, which comes
	// within a few seconds when it has nothing else to do
	m0 := len(shimMarked(mark, "failed close"))
	tried := false
	for deadline := time.Now().Add(15 * time.Second); time.Now().Before(deadline) && !tried; {
		time.Sleep(200 * time.Millisecond)
		tried = len(shimMarked(mark, "failed close")) > m0
	}
	if !tried {
		t.Errorf("the install was not tried again within 15 s while close() failed")
	}
	log := generalLog(s)
	if !strings.Contains(log, "Unable to close "+filepath.Join(s.Dir, ".accesslock")+" after writing its stamp") {
		t.Errorf("the failed close of the lock file is not logged as such")
	}
	if strings.Contains(log, "not announced") {
		t.Errorf("a failed close after a written stamp is logged as \"not announced\"")
	}
	if got := list(); !slices.Equal(got, []string{"e2e.other"}) {
		t.Errorf("with the stamp written but close() failing: groups %q, want B's [e2e.other] (the set is announced)", got)
	}

	if err := os.Remove(flag); err != nil {
		t.Fatal(err)
	}
	// the next try succeeds: an "Installing access cache" after the last failed close, and
	// none after that (a try that was under way when the fault went may still fail once)
	time.Sleep(time.Second)
	testkit.WaitFor(t, 15*time.Second, "a try after the fault", func() bool {
		log := generalLog(s)
		i := strings.LastIndex(log, "Unable to close ")
		return i >= 0 && strings.Contains(log[i:], "Installing access cache")
	})
	time.Sleep(time.Second)
	n1 := installs()
	time.Sleep(6 * time.Second)
	if n := installs(); n != n1 {
		t.Errorf("the install is still repeated after the fault is gone (%d more in 6 s)", n-n1)
	}
	if got := list(); !slices.Equal(got, []string{"e2e.other"}) {
		t.Errorf("after the fault: groups %q, want B's [e2e.other]", got)
	}
}

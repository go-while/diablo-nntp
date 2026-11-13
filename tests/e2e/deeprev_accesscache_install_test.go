package e2e

import (
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	testkit "github.com/go-while/nntp-testkit"
)

// Failure paths of InstallAccessCache() that were not retried, not paced or not noticed:
//
//   - a .new cache file that cannot be created ended the install after the file's identity
//     was recorded, so it was never tried again until dreader.access changed (A1);
//   - dreader.access that cannot be opened or statted was tried again on every pass of the
//     main loop, and a failed fstat() was not even logged (A3);
//   - a read error of dreader.access ended the parse like the end of the file, and the part
//     read was installed as the whole configuration (A4);
//   - a bad dreader.access that is not settled yet (mtime not older than the moment it was
//     read) was parsed again on every pass (A2).
//
// A pass of the main loop comes with every client connection, so each test hammers the
// reader with connections for about one retry period (5 s) and counts the log lines of the
// failure. The retry must come after 5 s, not on every pass.

// installFaultSource is a fault injector for the install of the access cache. While the file
// ACFAULT_FLAG exists:
//   - fgets() of a FILE whose name ends in ACFAULT_FGETS fails with a read error (the error
//     flag of the stream set, errno EIO) once ACFAULT_FGETS_AFTER lines were read from it;
//   - fchmod() of a descriptor whose name ends in one of the comma-separated suffixes of
//     ACFAULT_FCHMOD fails with EPERM;
//   - fclose() of a FILE whose name ends in ACFAULT_FCLOSE closes it and fails with ENOSPC (the
//     way a write error of a stream that is flushed at the close shows; write() under stdio
//     cannot be interposed).
//
// Each refused call is marked.
const installFaultSource = `#define _GNU_SOURCE
#define __EXTENSIONS__
#include <dlfcn.h>
#include <errno.h>
#include <fcntl.h>
#include <stdarg.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/types.h>
#include <sys/stat.h>
#include <unistd.h>
/*FDPATH*/
/*MARK*/
static int
ends(const char *path, const char *list)
{
    size_t n = strlen(path);

    while (list != NULL && *list) {
	const char *e = strchr(list, ',');
	size_t l = e ? (size_t)(e - list) : strlen(list);

	if (l > 0 && n >= l && strncmp(path + n - l, list, l) == 0)
	    return 1;
	list = e ? e + 1 : NULL;
    }
    return 0;
}

static int
flagged(void)
{
    const char *flag = getenv("ACFAULT_FLAG");

    return flag != NULL && access(flag, F_OK) == 0;
}

int
fchmod(int fd, mode_t mode)
{
    static int (*real)(int, mode_t);
    char path[4096];

    if (real == NULL)
	real = (int (*)(int, mode_t))dlsym(RTLD_NEXT, "fchmod");
    if (getenv("ACFAULT_FCHMOD") != NULL && flagged()) {
	fdpath(fd, path, sizeof(path));
	if (path[0] && ends(path, getenv("ACFAULT_FCHMOD"))) {
	    shim_mark("refused fchmod of %s", path);
	    errno = EPERM;
	    return -1;
	}
    }
    return real(fd, mode);
}

int
fclose(FILE *fp)
{
    static int (*real)(FILE *);
    char path[4096];
    int refuse = 0;
    int r;

    if (real == NULL)
	real = (int (*)(FILE *))dlsym(RTLD_NEXT, "fclose");
    if (fp != NULL && getenv("ACFAULT_FCLOSE") != NULL && flagged()) {
	fdpath(fileno(fp), path, sizeof(path));
	refuse = path[0] && ends(path, getenv("ACFAULT_FCLOSE"));
    }
    r = real(fp);
    if (refuse) {
	shim_mark("refused fclose of %s", path);
	errno = ENOSPC;
	return EOF;
    }
    return r;
}

static char *
fgets_fault(char *(*real)(char *, int, FILE *), char *s, int n, FILE *fp)
{
    static long lines;
    const char *suffix = getenv("ACFAULT_FGETS");
    const char *after = getenv("ACFAULT_FGETS_AFTER");
    char path[4096];

    if (suffix != NULL && fp != NULL && flagged()) {
	fdpath(fileno(fp), path, sizeof(path));
	if (path[0] && ends(path, suffix)) {
	    if (ftell(fp) == 0)
		lines = 0;
	    if (lines >= (after ? atol(after) : 0)) {
		shim_mark("refused fgets of %s after %ld lines", path, lines);
		(void)fputc('x', fp);	/* a write to a read-only stream sets its error flag */
		errno = EIO;
		return NULL;
	    }
	    lines++;
	}
    }
    return real(s, n, fp);
}

char *
fgets(char *s, int n, FILE *fp)
{
    static char *(*real)(char *, int, FILE *);

    if (real == NULL)
	real = (char *(*)(char *, int, FILE *))dlsym(RTLD_NEXT, "fgets");
    return fgets_fault(real, s, n, fp);
}

#ifdef __GLIBC__
char *
__fgets_chk(char *s, size_t size, int n, FILE *fp)
{
    static char *(*real)(char *, int, FILE *);

    (void)size;
    if (real == NULL)
	real = (char *(*)(char *, int, FILE *))dlsym(RTLD_NEXT, "fgets");
    return fgets_fault(real, s, n, fp);
}
#endif
`

// installSite writes a reader site with dreader.access access (accessSetA when empty) and, when
// shim is not empty, the fault injector built from it, with env added to the environment of
// dreaderd. It does not start dreaderd. The fault flag file and the mark file of the
// injector are in a directory of their own; the flag does not exist yet.
func installSite(t *testing.T, shim, access string, env ...string) (s *Site, flag, mark string) {
	t.Helper()
	if access == "" {
		access = accessSetA
	}
	dir := t.TempDir()
	flag = filepath.Join(dir, "fail")
	mark = filepath.Join(dir, "mark")
	_, spool := spoolStore(t)
	s = NewReaderSite(t, ReaderOptions{Spool: spool, Access: access, NoStart: true})
	if shim != "" {
		so := preloadShim(t, "installfault", withMark(withFdPath(shim)))
		s.Env = append(s.Env, lockFaultEnv(so))
	}
	s.Env = append(s.Env, "SHIM_MARK="+mark, "ACFAULT_FLAG="+flag)
	s.Env = append(s.Env, env...)
	t.Cleanup(func() {
		if t.Failed() {
			log := generalLog(s)
			if len(log) > 20000 {
				log = "...\n" + log[len(log)-20000:]
			}
			t.Logf("general.log:\n%s\nmarks:\n%s", log, strings.Join(shimMarks(mark), "\n"))
		}
	})
	return s, flag, mark
}

// startAndList starts dreaderd on a site whose dreader.access is valid, waits for the greeting
// and returns LIST ACTIVE as a function. The site must give the groups of accessSetA.
func startAndList(t *testing.T, s *Site) func() []string {
	t.Helper()
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
	return list
}

// hammer makes connections to addr from several goroutines for d (each one is a pass of the
// main loop of dreaderd) and returns how many were made.
func hammer(addr string, d time.Duration) int {
	var wg sync.WaitGroup
	var mu sync.Mutex
	n := 0
	end := time.Now().Add(d)
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for time.Now().Before(end) {
				readerGreeting(addr)
				mu.Lock()
				n++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	return n
}

// retryWindow is how long the tests hammer a failure: more than one retry period (5 s), not
// yet two.
const retryWindow = 8 * time.Second

// logLines counts the lines of general.log that contain all of subs.
func logLines(s *Site, subs ...string) int {
	n := 0
loop:
	for _, l := range strings.Split(generalLog(s), "\n") {
		for _, sub := range subs {
			if !strings.Contains(l, sub) {
				continue loop
			}
		}
		n++
	}
	return n
}

// wantTries checks the number of tries of a failure seen in retryWindow: the first and the
// retry after 5 s (a third one would be 5 s too early), not one per connection.
func wantTries(t *testing.T, what string, got, conns int) {
	t.Helper()
	t.Logf("%s: %d times in %v with %d connections", what, got, retryWindow, conns)
	if conns < 20 {
		t.Fatalf("only %d connections in %v: too few passes of the main loop for the test to mean anything", conns, retryWindow)
	}
	if got < 2 {
		t.Errorf("%s: %d times in %v with %d connections, want the first try and one retry after 5 s", what, got, retryWindow, conns)
	}
	if got > 3 {
		t.Errorf("%s: %d times in %v with %d connections, want one try per 5 s, not one per pass", what, got, retryWindow, conns)
	}
}

// openFiles returns the names of the files the process holds open (Linux, /proc) that contain
// one of subs; ok is false where the open files cannot be listed.
func openFiles(pid int, subs ...string) (names []string, ok bool) {
	if runtime.GOOS != "linux" {
		return nil, false
	}
	ents, err := os.ReadDir("/proc/" + strconv.Itoa(pid) + "/fd")
	if err != nil {
		return nil, false
	}
	for _, e := range ents {
		l, err := os.Readlink(filepath.Join("/proc", strconv.Itoa(pid), "fd", e.Name()))
		if err != nil {
			continue
		}
		for _, sub := range subs {
			if strings.Contains(l, sub) {
				names = append(names, l)
				break
			}
		}
	}
	return names, true
}

// noOpenSource fails the test when the main process holds dreader.access or a .new cache
// file open: a failed install must close what it opened.
func noOpenSource(t *testing.T, s *Site) {
	t.Helper()
	pid := readMainPid(t, s)
	if names, ok := openFiles(pid, "dreader.access", ".new"); ok && len(names) > 0 {
		t.Errorf("a failed install left files open in dreaderd %d: %q", pid, names)
	}
}

// waitListB waits for dreader.access B to be in use (e2e.other only).
func waitListB(t *testing.T, list func() []string, what string) {
	t.Helper()
	var got []string
	testkit.WaitFor(t, 15*time.Second, what, func() bool {
		got = list()
		return slices.Equal(got, []string{"e2e.other"})
	})
}

// wantListA fails the test when the clients do not get set A's groups.
func wantListA(t *testing.T, list func() []string, when string) {
	t.Helper()
	if got := list(); !slices.Equal(got, []string{"e2e.test"}) {
		t.Errorf("%s: groups %q, want the old set's [e2e.test]", when, got)
	}
}

// TestDeepRevAccessCacheCreateFails (A1): CreateAccessCache() fails for one of the five .new
// files (here: a directory has the name of the .new file, so open() fails with EISDIR, as
// it would with EMFILE or EACCES). The install used to end there with the file identity of
// dreader.access recorded, so it was never tried again: the new dreader.access B did not take
// effect until it was edited again. Each of the five files is the failing one in turn, so
// each branch closes what it opened. During the fault the clients keep set A (no mixed
// set); the install is tried again after 5 s, not on every pass; no file stays open; once
// the fault is gone B is installed without another edit of dreader.access.
func TestDeepRevAccessCacheCreateFails(t *testing.T) {
	for _, name := range accessCacheFiles {
		t.Run(name, func(t *testing.T) {
			s, _, _ := installSite(t, "", "")
			list := startAndList(t, s)
			blocker := filepath.Join(s.Dir, name+".new")
			if err := os.Mkdir(blocker, 0o755); err != nil {
				t.Fatal(err)
			}
			s.WriteFile(t, "dreader.access", accessSetB)
			n := hammer(s.Addr, retryWindow)
			wantTries(t, "Unable to create "+name+".new", logLines(s, "Unable to create", name+".new"), n)
			wantListA(t, list, "during the fault")
			noOpenSource(t, s)

			if err := os.Remove(blocker); err != nil {
				t.Fatal(err)
			}
			waitListB(t, list, "B to be installed once "+name+".new can be created, without another edit of dreader.access")
		})
	}
}

// TestDeepRevAccessCacheCreateModeFails (A1): CreateAccessCache() creates the file but cannot
// set its mode (fchmod() fails): the file is closed again and the install is tried again,
// as for any other failure to create a cache file. The injector refuses fchmod() of
// cache.readeraccess.new, the last file, so the four streams opened before it are closed too.
func TestDeepRevAccessCacheCreateModeFails(t *testing.T) {
	s, flag, mark := installSite(t, installFaultSource, "", "ACFAULT_FCHMOD=cache.readeraccess.new")
	list := startAndList(t, s)
	if err := os.WriteFile(flag, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	s.WriteFile(t, "dreader.access", accessSetB)
	n := hammer(s.Addr, retryWindow)
	wantTries(t, "Unable to create cache.readeraccess.new", logLines(s, "Unable to create", "cache.readeraccess.new"), n)
	if got := len(shimMarked(mark, "refused fchmod")); got < 2 {
		t.Errorf("the injector refused fchmod() %d times, want at least 2", got)
	}
	wantListA(t, list, "during the fault")
	noOpenSource(t, s)

	if err := os.Remove(flag); err != nil {
		t.Fatal(err)
	}
	waitListB(t, list, "B to be installed once the mode can be set")
}

// TestDeepRevAccessCacheSourceOpenFails (A3): dreader.access cannot be opened (it is gone).
// The install used to be tried on every pass of the main loop, one critical log line each. It
// is now tried again after 5 s. The old cache stays in use (no empty install, its files
// are untouched), and once dreader.access is back, an idle dreaderd picks it up by itself
// (no client connects before the new cache files and the stamp are there).
func TestDeepRevAccessCacheSourceOpenFails(t *testing.T) {
	s, _, _ := installSite(t, "", "")
	list := startAndList(t, s)
	before := fileIds(t, s.Dir, accessCacheFiles...)
	src := filepath.Join(s.Dir, "dreader.access")
	if err := os.Rename(src, src+".away"); err != nil {
		t.Fatal(err)
	}
	n := hammer(s.Addr, retryWindow)
	wantTries(t, "Unable to open dreader.access", logLines(s, "Unable to open", "dreader.access"), n)
	wantListA(t, list, "with dreader.access missing")
	if after := fileIds(t, s.Dir, accessCacheFiles...); !mapsEqual(before, after) {
		t.Errorf("the cache files changed while dreader.access was missing: %v, then %v", before, after)
	}
	noOpenSource(t, s)

	s.WriteFile(t, "dreader.access", accessSetB)
	lock := filepath.Join(s.Dir, ".accesslock")
	fi0, err := os.Stat(lock)
	if err != nil {
		t.Fatal(err)
	}
	testkit.WaitFor(t, 12*time.Second, "an idle dreaderd to install B and write a new stamp", func() bool {
		fi, err := os.Stat(lock)
		return err == nil && fi.Size() > 0 && fi.ModTime().After(fi0.ModTime())
	})
	waitListB(t, list, "B to be in use")
}

// TestDeepRevAccessCacheSourceMissingAtStart (A3): dreaderd starts without dreader.access and
// without an access cache. It is tried again after 5 s, not on every pass; nothing is
// installed (no empty cache, no stamp), so the clients are refused; and once dreader.access
// appears, an idle dreaderd installs it by itself.
func TestDeepRevAccessCacheSourceMissingAtStart(t *testing.T) {
	s, _, _ := installSite(t, "", "")
	if err := os.Remove(filepath.Join(s.Dir, "dreader.access")); err != nil {
		t.Fatal(err)
	}
	startReaderNoWait(t, s)
	testkit.WaitFor(t, 5*time.Second, "the failure to open dreader.access in the log", func() bool {
		return logLines(s, "Unable to open", "dreader.access") > 0
	})
	n := hammer(s.Addr, retryWindow)
	wantTries(t, "Unable to open dreader.access", logLines(s, "Unable to open", "dreader.access"), n)
	if g := readerGreeting(s.Addr); strings.HasPrefix(g, "20") {
		t.Errorf("without dreader.access the client got %q, want a refusal", strings.TrimSpace(g))
	}
	for name, id := range fileIds(t, s.Dir, accessCacheFiles...) {
		if id != "missing" {
			t.Errorf("%s exists without a dreader.access: an empty cache was installed", name)
		}
	}
	if fi, err := os.Stat(filepath.Join(s.Dir, ".accesslock")); err == nil && fi.Size() > 0 {
		t.Errorf("the lock file has a stamp (%d bytes) without a dreader.access", fi.Size())
	}

	s.WriteFile(t, "dreader.access", accessSetA)
	lock := filepath.Join(s.Dir, ".accesslock")
	testkit.WaitFor(t, 12*time.Second, "an idle dreaderd to install dreader.access and write the stamp", func() bool {
		fi, err := os.Stat(lock)
		return err == nil && fi.Size() > 0
	})
	testkit.WaitFor(t, 5*time.Second, "service", func() bool {
		return strings.HasPrefix(readerGreeting(s.Addr), "20")
	})
}

// TestDeepRevAccessCacheSourceFstatFails (A3): fstat() of the open dreader.access fails. It
// used to be tried on every pass and not logged. Now it is logged and tried again after
// 5 s; the old cache stays in use; once the fault is gone B is installed without another
// edit of dreader.access.
func TestDeepRevAccessCacheSourceFstatFails(t *testing.T) {
	s, flag, mark, list := accessFaultSite(t, "ACFAULT_FSTAT=dreader.access")
	if err := os.WriteFile(flag, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	s.WriteFile(t, "dreader.access", accessSetB)
	n := hammer(s.Addr, retryWindow)
	wantTries(t, "Unable to stat dreader.access", logLines(s, "Unable to stat", "dreader.access"), n)
	wantTries(t, "refused fstat of dreader.access", len(shimMarked(mark, "refused fstat of")), n)
	wantListA(t, list, "during the fault")
	noOpenSource(t, s)
	waitB(t, flag, list)
}

// TestDeepRevAccessCacheSourceIsDirectory (A4): dreader.access can be opened and statted but
// not read: it is a directory, so read() fails with EISDIR. The read error ended the parse
// like the end of the file, and the empty prefix was installed over the working cache, so
// every client was refused. Now nothing is installed, the old cache stays in use, and the
// read is tried again after 5 s; once dreader.access is a file again, it is installed.
func TestDeepRevAccessCacheSourceIsDirectory(t *testing.T) {
	s, _, _ := installSite(t, "", "")
	list := startAndList(t, s)
	before := fileIds(t, s.Dir, accessCacheFiles...)
	src := filepath.Join(s.Dir, "dreader.access")
	if err := os.Remove(src); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(src, 0o755); err != nil {
		t.Fatal(err)
	}
	n := hammer(s.Addr, retryWindow)
	wantTries(t, "Unable to read dreader.access", logLines(s, "Unable to read", "dreader.access"), n)
	wantListA(t, list, "while dreader.access cannot be read")
	if after := fileIds(t, s.Dir, accessCacheFiles...); !mapsEqual(before, after) {
		t.Errorf("the cache files changed while dreader.access could not be read: %v, then %v", before, after)
	}
	noOpenSource(t, s)

	if err := os.Remove(src); err != nil {
		t.Fatal(err)
	}
	s.WriteFile(t, "dreader.access", accessSetB)
	waitListB(t, list, "B to be installed once dreader.access can be read")
}

// accessCut1 is the first three lines of accessSetB (and of accessSetA): the vserverdef.
const accessCut1 = `vserverdef DEFAULT
    hostname ` + PathHost + `
end
`

// TestDeepRevAccessCacheSourceReadCut (A4): a read error in the middle of
// dreader.access (the injector fails fgets() with the stream's error flag set after n lines).
// The part read must not be installed, and it must not be judged either: a cut file is not
// a bad one, so a content error in the part read is not reported while the read fails. The
// old cache stays in use and the read is tried again after 5 s. Once the read works again, the
// complete file counts: B installs, or the content error in the file is reported.
func TestDeepRevAccessCacheSourceReadCut(t *testing.T) {
	cases := []struct {
		name    string
		content string
		after   int    // lines read before the failure
		verdict string // what the log says of the complete file once the read works; "" = B installs
		absent  string // what the log must not say while the read fails
	}{
		{name: "prefix", content: accessSetB, after: 3},
		{name: "badline",
			content: accessCut1 + "bogus directive\n" + strings.TrimPrefix(accessSetB, accessCut1),
			after:   5, verdict: "Errors in dreader.access file", absent: "Errors in dreader.access file"},
		{name: "nolist",
			content: accessCut1 + "readerdef rp\n    read yes\n    groups nosuchlist\nend\naccess 127.0.0.1 rp\n",
			after:   7, verdict: "is not a groupdef", absent: "is not a groupdef"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s, flag, mark := installSite(t, installFaultSource, "",
				"ACFAULT_FGETS=dreader.access", "ACFAULT_FGETS_AFTER="+strconv.Itoa(c.after))
			list := startAndList(t, s)
			if err := os.WriteFile(flag, nil, 0o644); err != nil {
				t.Fatal(err)
			}
			s.WriteFile(t, "dreader.access", c.content)
			n := hammer(s.Addr, retryWindow)
			wantTries(t, "Unable to read dreader.access", logLines(s, "Unable to read", "dreader.access"), n)
			if got := len(shimMarked(mark, "refused fgets")); got < 2 {
				t.Errorf("the injector refused fgets() %d times, want at least 2", got)
			}
			if c.absent != "" {
				if k := logLines(s, c.absent); k > 0 {
					t.Errorf("%d log lines %q while the file could not be read to its end: a cut file was judged", k, c.absent)
				}
			}
			wantListA(t, list, "while dreader.access cannot be read to its end")
			noOpenSource(t, s)

			if err := os.Remove(flag); err != nil {
				t.Fatal(err)
			}
			if c.verdict == "" {
				waitListB(t, list, "B to be installed once dreader.access can be read")
				return
			}
			testkit.WaitFor(t, 15*time.Second, "the verdict of the complete file in the log once the read works", func() bool {
				return logLines(s, c.verdict) > 0
			})
			wantListA(t, list, "with a bad dreader.access")
		})
	}
}

// unsettledParse runs the check of a bad dreader.access (maxconnperhost 1x) whose mtime is
// mtime ahead of the moment dreaderd reads it, so it is not settled: a same-second fix keeps
// mtime, size and inode. The refused file used to be parsed on every pass of the main
// loop: under load, thousands of times a second. It is now parsed again after 5 s, not on
// every pass. The fix, a value of the same length written in place with the same mtime, is
// picked up at that retry although the file's identity did not change (and, for a small
// mtime, although the file has aged by then).
func unsettledParse(t *testing.T, ahead time.Duration) {
	s, _, _ := installSite(t, "", accessWith("maxconnperhost 1x"))
	path := filepath.Join(s.Dir, "dreader.access")
	mtime := time.Now().Add(ahead)
	if err := os.Chtimes(path, mtime, mtime); err != nil {
		t.Fatal(err)
	}
	startReaderNoWait(t, s)
	testkit.WaitFor(t, 5*time.Second, "the bad-value line in the log", func() bool {
		return strings.Contains(generalLog(s), "bad value")
	})
	n := hammer(s.Addr, 2500*time.Millisecond)
	if n < 20 {
		t.Fatalf("only %d connections in 2.5 s: too few passes of the main loop for the test to mean anything", n)
	}
	t.Logf("%d parses of the bad file in 2.5 s with %d connections", logLines(s, "Errors in dreader.access file"), n)
	if k := logLines(s, "Errors in dreader.access file"); k > 2 {
		t.Errorf("the unsettled bad file was parsed %d times in 2.5 s with %d connections, want at most 2 (paced)", k, n)
	}

	pid := readMainPid(t, s)
	proc, err := os.FindProcess(pid)
	if err != nil {
		t.Fatal(err)
	}
	if err := proc.Signal(syscall.SIGSTOP); err != nil {
		t.Fatalf("SIGSTOP dreaderd %d: %v", pid, err)
	}
	defer proc.Signal(syscall.SIGCONT)
	if err := os.WriteFile(path, []byte(accessWith("maxconnperhost 10")), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, mtime, mtime); err != nil {
		t.Fatal(err)
	}
	if err := proc.Signal(syscall.SIGCONT); err != nil {
		t.Fatalf("SIGCONT dreaderd %d: %v", pid, err)
	}
	testkit.WaitFor(t, 15*time.Second, "service to resume after the same-second in-place fix", func() bool {
		g := readerGreeting(s.Addr)
		return strings.HasPrefix(g, "200") || strings.HasPrefix(g, "201")
	})
}

// TestDeepRevAccessCacheUnsettled (A2): see unsettledParse.
func TestDeepRevAccessCacheUnsettled(t *testing.T) {
	t.Run("future", func(t *testing.T) { unsettledParse(t, 60*time.Second) })
	t.Run("ages", func(t *testing.T) { unsettledParse(t, 2*time.Second) })
}

// TestDeepRevAccessCacheSettledBadWithWriteFault: the precedence of content over output
// failure stays: a settled bad dreader.access (older than the moment it was read) is
// refused once, even when the output of the .new files fails as well (the fclose() of
// cache.groupdef.new reports ENOSPC). It is neither parsed again on every pass nor rebuilt after 5 s as
// a failed output would be, and the failed write is not what the log reports.
func TestDeepRevAccessCacheSettledBadWithWriteFault(t *testing.T) {
	s, flag, mark := installSite(t, installFaultSource, accessWith("maxconnperhost 1x"),
		"ACFAULT_FCLOSE=cache.groupdef.new")
	if err := os.WriteFile(flag, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-10 * time.Second)
	if err := os.Chtimes(filepath.Join(s.Dir, "dreader.access"), old, old); err != nil {
		t.Fatal(err)
	}
	startReaderNoWait(t, s)
	testkit.WaitFor(t, 5*time.Second, "the refusal of the bad file in the log", func() bool {
		return logLines(s, "Errors in dreader.access file") > 0
	})
	n := hammer(s.Addr, retryWindow)
	if n < 20 {
		t.Fatalf("only %d connections in %v: too few passes of the main loop for the test to mean anything", n, retryWindow)
	}
	if got := len(shimMarked(mark, "refused fclose of")); got == 0 {
		t.Fatalf("the injector refused no fclose() of cache.groupdef.new: the output fault was not in place")
	}
	if k := logLines(s, "Errors in dreader.access file"); k != 1 {
		t.Errorf("the settled bad file was refused %d times, want once", k)
	}
	if k := logLines(s, "Unable to write the new access cache files"); k != 0 {
		t.Errorf("the failed output was reported %d times for a file refused for its content", k)
	}
}

func mapsEqual(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

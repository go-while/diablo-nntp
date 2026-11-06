package e2e

// Robustness of the feeder history (lib/history.c, util/diablo.c): a file that
// ends inside a record, a chain that points at an unreadable record, and a
// history that cannot take a new record.

import (
	"fmt"
	"github.com/go-while/nntp-testkit/client"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// histDrvBuild compiles histDriver against the library of the tree under test
// (E2E_SRC, else the tree of DIABLO_BIN; it must have been built) and returns
// the executable.
func histDrvBuild(t *testing.T) string {
	t.Helper()
	cc := wmCC(t)
	root := wildmatSource(t)
	if _, err := os.Stat(filepath.Join(root, "obj", "libdiablo.a")); err != nil {
		t.Skipf("no built library in %s: %v", root, err)
	}
	dir := t.TempDir()
	c := filepath.Join(dir, "histdrv.c")
	if err := os.WriteFile(c, []byte(histDriver), 0o644); err != nil {
		t.Fatal(err)
	}
	exe := filepath.Join(dir, "histdrv")
	// The flags and libraries the Makefile builds libdiablo.a with: the driver
	// must agree with it on off_t (a History record holds one) and link what it
	// needs. config.mk lists the libraries of the optional features.
	cflags := []string{"-O1", "-g", "-D_FILE_OFFSET_BITS=64"}
	libs := []string{"-lm", "-lcrypto"}
	switch runtime.GOOS {
	case "illumos", "solaris":
		cflags = append(cflags, "-D_LARGEFILE_SOURCE", "-D_XOPEN_SOURCE=600", "-D__EXTENSIONS__")
		libs = append(libs, "-lnsl", "-lsocket", "-lposix4", "-lmd")
	case "freebsd":
		libs = append(libs, "-lmd")
	}
	if b, err := os.ReadFile(filepath.Join(root, "config.mk")); err == nil {
		for _, l := range strings.Split(string(b), "\n") {
			if v, ok := strings.CutPrefix(strings.TrimSpace(l), "LDLIBS +="); ok {
				libs = append(libs, strings.Fields(v)...)
			}
		}
	}
	args := append(append(cflags, "-I"+filepath.Join(root, "lib"), "-I"+root,
		"-o", exe, c, "-L"+filepath.Join(root, "obj"), "-ldiablo"), libs...)
	if out, err := exec.Command(cc, args...).CombinedOutput(); err != nil {
		t.Fatalf("compiling the history driver: %v\n%s", err, out)
	}
	return exe
}

func histDrvRun(t *testing.T, exe, mode string) map[string]bool {
	t.Helper()
	cmd := exec.Command(exe, mode, filepath.Join(t.TempDir(), "hist.t"))
	done := make(chan struct{})
	var out []byte
	var err error
	go func() { out, err = cmd.CombinedOutput(); close(done) }()
	select {
	case <-done:
	case <-time.After(60 * time.Second):
		_ = cmd.Process.Kill()
		<-done
		t.Fatalf("driver %s timed out\n%s", mode, out)
	}
	t.Logf("%s:\n%s", mode, out)
	if err != nil {
		t.Fatalf("driver %s: %v", mode, err)
	}
	lines := map[string]bool{}
	for _, l := range strings.Split(string(out), "\n") {
		lines[l] = true
	}
	return lines
}

func histDrvWant(t *testing.T, lines map[string]bool, want ...string) {
	t.Helper()
	for _, w := range want {
		if !lines[w] {
			t.Errorf("missing %q", w)
		}
	}
}

// TestHistoryAddAfterPartialRecord: a history file that ends inside a record
// (a crash in the middle of an append) must not skew the index of every
// later record. After 5 stray bytes at the end, new articles are found with
// their own hash, a second add of one is refused as a duplicate, and the
// file is again a whole number of records.
// Fails on main: the chain index points up to 27 bytes before the record, so
// lookups read garbage and the second add of the same Message-ID is accepted.
func TestHistoryAddAfterPartialRecord(t *testing.T) {
	exe := histDrvBuild(t)
	lines := histDrvRun(t, exe, "partial")
	histDrvWant(t, lines, "a found", "b found with its hash", "c found with its hash",
		"re-add b refused", "never added is a miss", "file is whole records")
}

// TestHistoryLookupBadChainIsMiss: a chain index that points past the end of
// the file (or at a record that cannot be read) ends the chain walk as "not
// found" in HistoryLookup, HistoryLookupByHash and HistoryPosLookupByHash;
// a new article of that bucket is added and then found, and the record of
// another bucket is untouched.
// Fails on main: all three report a hit, so every new article of the bucket
// is refused as a duplicate and HistoryStoreExp writes at the bad offset.
func TestHistoryLookupBadChainIsMiss(t *testing.T) {
	exe := histDrvBuild(t)
	lines := histDrvRun(t, exe, "badchain")
	histDrvWant(t, lines, "lookup miss", "lookup by hash miss", "pos lookup miss",
		"add accepted", "added is found", "other bucket intact", "file size unchanged by lookups")
}

// histFailSource is an LD_PRELOAD shim: while the file HISTFAIL_FLAG exists,
// pwrite() to a file called .../dhistory fails with ENOSPC.
const histFailSource = `#define _GNU_SOURCE
#define __EXTENSIONS__
#include <dlfcn.h>
#include <errno.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>
#include <sys/types.h>
#include <fcntl.h>
/*FDPATH*/
static int
fail(int fd)
{
    const char *flag = getenv("HISTFAIL_FLAG");
    char path[4096];
    size_t n;

    if (flag == NULL || access(flag, F_OK) != 0)
	return 0;
    fdpath(fd, path, sizeof(path));
    n = strlen(path);
    return n >= 9 && strcmp(path + n - 9, "/dhistory") == 0;
}

ssize_t
pwrite(int fd, const void *buf, size_t len, off_t off)
{
    ssize_t (*real)(int, const void *, size_t, off_t) =
	(ssize_t (*)(int, const void *, size_t, off_t))dlsym(RTLD_NEXT, "pwrite");

    if (fail(fd)) {
	errno = ENOSPC;
	return -1;
    }
    return real(fd, buf, len, off);
}

#ifdef __linux__
ssize_t
pwrite64(int fd, const void *buf, size_t len, off_t off)
{
    ssize_t (*real)(int, const void *, size_t, off_t) =
	(ssize_t (*)(int, const void *, size_t, off_t))dlsym(RTLD_NEXT, "pwrite64");

    if (fail(fd)) {
	errno = ENOSPC;
	return -1;
    }
    return real(fd, buf, len, off);
}
#endif
`

// TestFeederHistoryWriteFailureTryAgain: when the history cannot take the
// record of a received article (ENOSPC on the history volume), the peer is
// told to try again later (436 to IHAVE, 431 to TAKETHIS) instead of being
// refused for good (437 / 439), and once the history works again the same
// articles are taken.
// Fails on main: HistoryAdd's RCTRYAGAIN became RCREJECT, so 437 and 439.
func TestFeederHistoryWriteFailureTryAgain(t *testing.T) {
	if !preloadHost(false) {
		t.Skip("the fault injector uses LD_PRELOAD")
	}
	so := preloadShim(t, "histfail", histFailSource)
	flag := filepath.Join(t.TempDir(), "histfail")
	s := NewSite(t, SiteOptions{
		Config: map[string]string{"path_generallog": "%s/general.log"},
		Setup: func(tb testing.TB, s *Site) {
			s.Env = append(s.Env, lockFaultEnv(so), "HISTFAIL_FLAG="+flag)
		},
	})
	p := s.Peer(t)
	if err := os.WriteFile(flag, nil, 0o644); err != nil {
		t.Fatal(err)
	}

	ihave := msgID(t)
	art := newArticle(ihave, "e2e.test", "history full ihave", "body")
	p.ExpectCmd(335, "IHAVE %s", ihave)
	p.MustSend(client.Stuff(art))
	p.ExpectStatus(436, "IHAVE with a failing history")

	p.ExpectCmd(203, "MODE STREAM")
	take := msgID(t)
	takeArt := client.Stuff(newArticle(take, "e2e.test", "history full takethis", "body"))
	if err := p.Send(fmt.Sprintf("TAKETHIS %s\r\n%s", take, takeArt)); err != nil {
		t.Fatal(err)
	}
	p.ExpectStatus(431, "TAKETHIS with a failing history")

	if err := os.Remove(flag); err != nil {
		t.Fatal(err)
	}
	p.ExpectCmd(335, "IHAVE %s", ihave)
	p.MustSend(client.Stuff(art))
	p.ExpectStatus(235, "IHAVE after the history works again")
	if err := p.Send(fmt.Sprintf("TAKETHIS %s\r\n%s", take, takeArt)); err != nil {
		t.Fatal(err)
	}
	p.ExpectStatus(239, "TAKETHIS after the history works again")
	p.ExpectCmd(223, "STAT %s", ihave)
	p.ExpectCmd(223, "STAT %s", take)
	p.ExpectCmd(205, "QUIT")
}

const histDriver = `#include "defs.h"
#include <sys/stat.h>

int VerboseOpt;
extern off_t HEntryOff;

static History
mk(const char *id)
{
    History h;

    memset(&h, 0, sizeof(h));
    h.hv = hhash(id);
    h.gmt = 100;
    h.exp = 101;
    h.boffset = 1;
    h.bsize = 10;
    return h;
}

static int
add(const char *id)
{
    History h = mk(id);

    return HistoryAdd(id, &h);
}

/* found with its own hash */
static int
foundOwn(const char *id)
{
    History h;
    hash_t hv = hhash(id);

    return HistoryLookup(id, &h) == 0 && h.hv.h1 == hv.h1 && h.hv.h2 == hv.h2;
}

static void
stray(const char *file, int n)
{
    int fd = open(file, O_WRONLY | O_APPEND);
    if (fd < 0 || write(fd, "123456789012345678901234567", n) != n) {
	perror("stray");
	exit(1);
    }
    close(fd);
}

static void
say(int ok, const char *what)
{
    if (ok)
	printf("%s\n", what);
    else
	printf("FAILED %s\n", what);
}

int
main(int ac, char **av)
{
    const char *mode = ac > 2 ? av[1] : "";
    const char *file = ac > 2 ? av[2] : "hist.t";
    struct stat st;

    unlink(file);
    DOpts.HashSize = 262144;
    if (HistoryOpen(file, 0) < 0) {
	printf("open failed\n");
	return 1;
    }
    if (strcmp(mode, "partial") == 0) {
	say(add("<a@x>") == RCOK && foundOwn("<a@x>"), "a found");
	stray(file, 5);
	add("<b@x>");
	add("<c@x>");
	say(foundOwn("<b@x>"), "b found with its hash");
	say(foundOwn("<c@x>"), "c found with its hash");
	say(add("<b@x>") == RCALREADY, "re-add b refused");
	say(HistoryLookup("<never@x>", NULL) < 0, "never added is a miss");
	stat(file, &st);
	say((st.st_size - HEntryOff) % sizeof(History) == 0, "file is whole records");
	HistoryClose();
	return 0;
    }
    if (strcmp(mode, "badchain") == 0) {
	History h, junk;
	HistHead hh;
	hash_t hz = hhash("<z@x>"), ha = hhash("<a@x>");
	HistIndex bad = 0x7ffffff0;
	int fd;
	off_t pos;
	off_t size0;

	add("<a@x>");
	HistoryClose();
	fd = open(file, O_RDWR);
	if (fd < 0 || pread(fd, &hh, sizeof(hh), 0) != sizeof(hh)) {
	    printf("header read failed\n");
	    return 1;
	}
	pos = hh.headSize + (off_t)((hz.h1 ^ hz.h2) & (hh.hashSize - 1)) * sizeof(HistIndex);
	if (pwrite(fd, &bad, sizeof(bad), pos) != sizeof(bad)) {
	    printf("index write failed\n");
	    return 1;
	}
	fstat(fd, &st);
	size0 = st.st_size;
	close(fd);
	DOpts.HashSize = hh.hashSize;
	if (HistoryOpen(file, 0) < 0) {
	    printf("reopen failed\n");
	    return 1;
	}
	if (((ha.h1 ^ ha.h2) & (hh.hashSize - 1)) == ((hz.h1 ^ hz.h2) & (hh.hashSize - 1))) {
	    printf("test hashes collide, choose other ids\n");
	    return 1;
	}
	say(HistoryLookup("<z@x>", &h) < 0, "lookup miss");
	say(HistoryLookupByHash(hz, &junk) < 0, "lookup by hash miss");
	say(HistoryPosLookupByHash(hz, &junk) == (HistIndex)-1, "pos lookup miss");
	fd = open(file, O_RDONLY);
	fstat(fd, &st);
	close(fd);
	say(st.st_size == size0, "file size unchanged by lookups");
	say(add("<z@x>") == RCOK, "add accepted");
	say(foundOwn("<z@x>"), "added is found");
	say(foundOwn("<a@x>"), "other bucket intact");
	HistoryClose();
	return 0;
    }
    printf("unknown mode\n");
    return 1;
}
`

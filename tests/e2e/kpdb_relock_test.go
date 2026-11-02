package e2e

// KPDBLookup(..., KP_LOCK) in lib/kpdb.c finds a record and then waits for
// its lock.  If another process replaces the record meanwhile (a write that
// changes the record's length appends a new copy and marks the old one
// deleted), the waiter used to come back holding the lock of the deleted
// copy and read its stale fields.  GenerateXRef() in lib/active.c reads NX
// (or NE) under that lock and writes NX+1, so the feeder handed out the
// same article number twice, for example when a group's first NX was
// added.  The fix looks the record up again after the wait when it is no
// longer live.
//
// lib/kpdb.c is compiled unchanged with a small stand-in for defs.h and an
// hflock() that, in the second process only, reports through a pipe when it
// is about to wait for a record lock.  By then that process has found the
// record, so the first process replaces it exactly inside the window; no
// sleeps.  Cases:
//
//	replace1  P1 holds the record and adds the first NX (append path) while
//	          P2 waits; P2 must allocate 2, not 1 again.
//	replace3  P1 replaces the record three times (adding a field each time,
//	          keeping the new copy locked), so P2 has to look again three
//	          times before it gets the live record.
//	delete    P1 deletes the record while P2 waits; P2 must see no record,
//	          not the deleted copy.
//	badstatus a sorted record whose status byte is neither '+' nor '-' (a
//	          damaged file) is not found, rather than returned (main) or
//	          looked up again forever (the first version of the fix).
//
// Needs a C compiler and the source tree; skips otherwise, and fails instead
// when E2E_REQUIRE_SRC=1 is set (see wmSkipf in wildmat_unit_test.go).

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestKPDBLockRelookup(t *testing.T) {
	cc := wmCC(t)
	root := wildmatSource(t) // the tree root; lib/kpdb.c sits beside lib/wildmat.c
	dir := t.TempDir()
	for _, f := range []string{"kpdb.c", "kpdb.h"} {
		b, err := os.ReadFile(filepath.Join(root, "lib", f))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, f), b, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// kpdb.c includes "defs.h" with quotes, so the copy beside it is used.
	if err := os.WriteFile(filepath.Join(dir, "defs.h"), []byte(kpRelockDefs), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "driver.c"), []byte(kpRelockDriver), 0o644); err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(dir, "kprelock")
	cmd := exec.Command(cc, "-O1", "-g", "-std=gnu99", "-D_FILE_OFFSET_BITS=64",
		"-o", bin, filepath.Join(dir, "kpdb.c"), filepath.Join(dir, "driver.c"))
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("compiling the kpdb driver: %v\n%s", err, out)
	}

	for _, tc := range []struct {
		name string
		want []string
	}{
		{"replace1", []string{"P1 assigned 1", "P2 assigned 2", "final NX=0000000002 live=1"}},
		{"replace3", []string{"P1 assigned 1", "P2 waited 4 times", "P2 assigned 2", "final NX=0000000002 live=1"}},
		{"delete", []string{"P1 deleted", "P2 no record", "final live=0"}},
		{"badstatus", []string{"badstatus no record"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			db := filepath.Join(t.TempDir(), "active.kp")
			out, err := exec.CommandContext(ctx, bin, tc.name, db).CombinedOutput()
			if err != nil {
				t.Fatalf("driver %s: %v\n%s", tc.name, err, out)
			}
			var problems []string
			for _, w := range tc.want {
				if !strings.Contains(string(out), w+"\n") {
					problems = append(problems, fmt.Sprintf("no %q", w))
				}
			}
			if strings.Contains(string(out), "holds a deleted record") {
				problems = append(problems, "P2 read the deleted copy")
			}
			if len(problems) > 0 {
				t.Errorf("%s:\n%s", strings.Join(problems, "; "), out)
			}
		})
	}
}

// kpRelockDefs stands in for lib/defs.h: what lib/kpdb.c uses, and nothing
// that needs the rest of the library.
const kpRelockDefs = `#include <sys/types.h>
#include <sys/stat.h>
#include <sys/mman.h>
#include <errno.h>
#include <fcntl.h>
#include <limits.h>
#include <stdarg.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <strings.h>
#include <syslog.h>
#include <time.h>
#include <unistd.h>

typedef int int32;
typedef unsigned int uint32;
typedef unsigned char uint8;
typedef struct MemPool MemPool;

#define Prototype extern
#define XLOCK_SH	1
#define XLOCK_EX	2
#define XLOCK_UN	3
#define XLOCK_NB	0x10

#include "kpdb.h"

extern MemPool *SysMemPool;
extern volatile int DebugOpt;
void *zalloc(MemPool **mp, int bytes);
void zfree(MemPool **mp, void *ptr, int bytes);
void *xmap(void *addr, size_t len, int prot, int flags, int fd, off_t offset);
void xunmap(void *addr, size_t len);
void logit(int priority, const char *ctl, ...);
int hflock(int fd, off_t offset, int flags);
void *pagealloc(int *psize, int bytes);
void pagefree(void *p, int bytes);
`

const kpRelockDriver = `#include "defs.h"
#include <signal.h>
#include <sys/wait.h>

KPDB *KPDBOpen(const char *fileName, int oflags);
void KPDBClose(KPDB *kpdb);
const char *KPDBReadRecord(KPDB *kpdb, const char *key, int lockMe, int *preclen);
const char *KPDBGetField(const char *rec, int recLen, const char *tok, int *plen, const char *def);
int KPDBWrite(KPDB *kpdb, const char *key, const char *tok, const char *data, int lockMe);
int KPDBDelete(KPDB *kpdb, const char *key);
int KPDBReadRecordOff(KPDB *kpdb, const char *key, int lockMe, int *preclen);
void KPDBReSort(KPDB *kpdb);

MemPool *SysMemPool;
volatile int DebugOpt;

void *zalloc(MemPool **mp, int bytes) { (void)mp; return calloc(1, bytes); }
void zfree(MemPool **mp, void *ptr, int bytes) { (void)mp; (void)bytes; free(ptr); }
void logit(int priority, const char *ctl, ...) { (void)priority; (void)ctl; }
void *pagealloc(int *psize, int bytes) { *psize = bytes; return calloc(1, bytes); }
void pagefree(void *p, int bytes) { (void)bytes; free(p); }

void *
xmap(void *addr, size_t len, int prot, int flags, int fd, off_t offset)
{
    void *p = mmap(addr, len ? len : 1, prot, flags, fd, offset);
    return (p == MAP_FAILED) ? NULL : p;
}

void xunmap(void *addr, size_t len) { munmap(addr, len ? len : 1); }

/*
 * hflock() as in lib/lock.c, but when HookFd is set (in P2) it reports a
 * blocking wait for a record lock (offsets 0 and 4 are the open and append
 * locks) before it waits.  The lookup has found the record by then.
 */
static int HookFd = -1;
static int Waits;

int
hflock(int fd, off_t offset, int flags)
{
    struct flock fl;

    memset(&fl, 0, sizeof(fl));
    switch (flags & 0x0F) {
    case XLOCK_SH: fl.l_type = F_RDLCK; break;
    case XLOCK_EX: fl.l_type = F_WRLCK; break;
    case XLOCK_UN: fl.l_type = F_UNLCK; break;
    }
    fl.l_whence = SEEK_SET;
    fl.l_start = offset;
    fl.l_len = 4;
    if (HookFd >= 0 && (flags & 0x0F) == XLOCK_EX && !(flags & XLOCK_NB) &&
								offset >= 8) {
	long long o = (long long)offset;

	++Waits;
	if (write(HookFd, &o, sizeof(o)) != sizeof(o))
	    _exit(3);
    }
    return fcntl(fd, (flags & XLOCK_NB) ? F_SETLK : F_SETLKW, &fl);
}

static const char *Key = "alt.test";

/* GenerateXRef(): lock the record, n = NX+1 (or NE+1), write NX, unlock. */
static long long
allocNumber(KPDB *db, const char *who)
{
    int recLen, len;
    const char *rec = KPDBReadRecord(db, Key, KP_LOCK, &recLen);
    const char *f;
    long long n;
    char buf[32];

    if (rec == NULL) {
	printf("%s no record\n", who);
	return -1;
    }
    if (rec[0] != '+')
	printf("%s holds a deleted record\n", who);
    if ((f = KPDBGetField(rec, recLen, "NX", &len, NULL)) == NULL)
	f = KPDBGetField(rec, recLen, "NE", &len, "0");
    n = strtoll(f, NULL, 10) + 1;
    snprintf(buf, sizeof(buf), "%010lld", n);
    KPDBWrite(db, Key, "NX", buf, KP_UNLOCK);
    printf("%s assigned %lld\n", who, n);
    return n;
}

static void
lookupHung(int sig)
{
    (void)sig;
    printf("badstatus lookup did not return\n");
    _exit(1);
}

/*
 * A damaged status byte on a sorted record: the binary search reaches it.
 * The lookup must report no record and return.
 */
static int
badStatus(const char *path)
{
    KPDB *db;
    const char *rec;
    int recLen, off, fd;

    if ((db = KPDBOpen(path, O_RDWR)) == NULL)
	return 2;
    KPDBReSort(db);
    off = KPDBReadRecordOff(db, Key, 0, &recLen);
    KPDBClose(db);
    if (off <= 0 || (fd = open(path, O_RDWR)) < 0 ||
					pwrite(fd, "*", 1, off) != 1)
	return 2;
    close(fd);

    if ((db = KPDBOpen(path, O_RDWR)) == NULL)
	return 2;
    signal(SIGALRM, lookupHung);
    alarm(10);
    rec = KPDBReadRecord(db, Key, KP_LOCK, &recLen);
    alarm(0);
    if (rec == NULL)
	printf("badstatus no record\n");
    else
	printf("badstatus found a record with status '%c'\n", rec[0]);
    return 0;
}

int
main(int ac, char **av)
{
    const char *mode, *path;
    int go[2], hook[2], ready[2];
    int rounds = 1, del = 0, i, st, recLen;
    pid_t p2;
    KPDB *db;
    const char *rec;
    long long off;

    if (ac != 3)
	return 2;
    mode = av[1];
    path = av[2];
    if (strcmp(mode, "replace3") == 0)
	rounds = 3;
    else if (strcmp(mode, "delete") == 0)
	del = 1;
    else if (strcmp(mode, "replace1") != 0 && strcmp(mode, "badstatus") != 0)
	return 2;
    setvbuf(stdout, NULL, _IONBF, 0);

    /* A group as the feeder sees it before its first article: no NX yet. */
    unlink(path);
    if ((db = KPDBOpen(path, O_RDWR|O_CREAT)) == NULL)
	return 2;
    KPDBWrite(db, "alt.aaa", "NB", "0000000001", 0);
    KPDBWrite(db, Key, "NB", "0000000001", KP_LOCK);
    KPDBWrite(db, Key, "NE", "0000000000", KP_LOCK_CONTINUE);
    KPDBWrite(db, Key, "S", "y", KP_UNLOCK);
    KPDBWrite(db, "alt.zzz", "NB", "0000000001", 0);
    KPDBClose(db);
    if (strcmp(mode, "badstatus") == 0)
	return badStatus(path);

    if (pipe(go) < 0 || pipe(hook) < 0 || pipe(ready) < 0)
	return 2;
    if ((p2 = fork()) == 0) {
	char c;
	KPDB *d2;

	close(go[1]);
	close(hook[0]);
	close(ready[0]);
	if ((d2 = KPDBOpen(path, O_RDWR)) == NULL)
	    _exit(2);
	if (write(ready[1], "r", 1) != 1)
	    _exit(2);
	if (read(go[0], &c, 1) != 1)
	    _exit(2);
	HookFd = hook[1];
	allocNumber(d2, "P2");
	if (Waits > 1)
	    printf("P2 waited %d times\n", Waits);
	_exit(0);
    }
    close(go[0]);
    close(hook[1]);
    close(ready[1]);

    /* open one after the other, so a sort on open cannot overlap a lookup */
    if (read(ready[0], &off, 1) != 1)
	return 2;
    db = KPDBOpen(path, O_RDWR);
    if ((rec = KPDBReadRecord(db, Key, KP_LOCK, &recLen)) == NULL)
	return 2;
    if (write(go[1], "g", 1) != 1)
	return 2;

    /*
     * Each round: wait until P2 has found the record and is about to wait
     * for its lock, then replace (or delete) the record under it.  A P2
     * that does not look again never reports a second wait; it finishes,
     * and the pipe reads EOF.
     */
    for (i = 1; i <= rounds; ++i) {
	if (read(hook[0], &off, sizeof(off)) != sizeof(off)) {
	    printf("P2 did not look again (round %d)\n", i);
	    break;
	}
	if (del) {
	    KPDBDelete(db, Key);	/* we hold the lock; this releases it */
	    printf("P1 deleted\n");
	} else if (i < rounds) {
	    char tok[8];

	    /* a new field: append path; the new copy stays locked */
	    snprintf(tok, sizeof(tok), "X%d", i);
	    KPDBWrite(db, Key, tok, "1", KP_LOCK_CONTINUE);
	} else {
	    /* the group's first NX: append path, then unlock */
	    KPDBWrite(db, Key, "NX", "0000000001", KP_UNLOCK);
	    printf("P1 assigned 1\n");
	}
    }
    if (i <= rounds && !del)
	KPDBWrite(db, Key, "NX", "0000000001", KP_UNLOCK);
    if (waitpid(p2, &st, 0) != p2 || !WIFEXITED(st) || WEXITSTATUS(st) != 0) {
	printf("P2 failed\n");
	return 1;
    }
    /* drain any later reports so a stale one cannot confuse the count */
    while (read(hook[0], &off, sizeof(off)) == sizeof(off))
	;

    {
	FILE *fp = fopen(path, "r");
	char line[1024];
	int live = 0;
	char nx[11] = "";

	if (fp == NULL)
	    return 2;
	while (fgets(line, sizeof(line), fp)) {
	    char *k = strchr(line, ':');
	    char *p;

	    if (line[0] != '+' || k == NULL || strncmp(k + 1, Key, strlen(Key)) != 0 ||
						k[1 + strlen(Key)] != ' ')
		continue;
	    ++live;
	    if ((p = strstr(line, " NX=")) != NULL)
		snprintf(nx, sizeof(nx), "%.10s", p + 4);
	}
	fclose(fp);
	if (del)
	    printf("final live=%d\n", live);
	else
	    printf("final NX=%s live=%d\n", nx, live);
    }
    return 0;
}
`

// TestOverToolsGroupRemovedWhileWaiting: dexpireover and doverctl read a
// group's record with KP_LOCK while they work through its overview files.
// When the group is removed from dactive.kp while they wait for that record
// (or before they look it up), the lookup now returns no record.  They used
// to take the missing NB/NE as -1: doverctl removed the group's data file as
// out of range, and dexpireover -s resized the index for an empty range,
// copying no entries.  They must skip the group and leave its files alone.
//
// The test holds the record's lock (a 4-byte fcntl lock at its offset, as
// hflock() takes it), starts the tool, waits until it is blocked on that lock
// (seen in /proc/locks; a fixed second where there is none), marks the record
// deleted as KPDBDelete() does, and releases the lock.
func TestOverToolsGroupRemovedWhileWaiting(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
	}{
		{"doverctl", []string{"doverctl", "-v", "clean"}},
		{"dexpireover", []string{"dexpireover", "-s", "-y", "-v"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, home := overSite(t, map[string]string{"readergrouphash": "hierarchy"})
			s.WriteFile(t, "dexpire.ctl", "e2e.*:x30\n")
			over := filepath.Join(home, "e2e", "o.0.e2e.test")
			data := filepath.Join(home, "e2e", "d.256.e2e.test")
			// 500 empty OverArt slots: the right size for NB..NE 1..300, so
			// -s resizes it only when the range is lost (-1).
			index := append(overHeadMax("e2e.test", 500), make([]byte, 500*32)...)
			writeOver(t, over, index)
			writeOver(t, data, []byte("xxxxxxxxxx"))

			db := filepath.Join(s.Dir, "dactive.kp")
			off := kpRecordOffset(t, db, "e2e.test")
			f, err := os.OpenFile(db, os.O_RDWR, 0)
			if err != nil {
				t.Fatal(err)
			}
			defer f.Close()
			lk := syscall.Flock_t{Type: syscall.F_WRLCK, Whence: 0, Start: off, Len: 4}
			if err := syscall.FcntlFlock(f.Fd(), syscall.F_SETLK, &lk); err != nil {
				t.Fatalf("lock record e2e.test: %v", err)
			}

			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, filepath.Join(s.Bin, tc.args[0]), tc.args[1:]...)
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
				t.Fatalf("%s did not wait for the record: %v\n%s", tc.args[0], err, out.String())
			}
			if _, err := f.WriteAt([]byte("-"), off); err != nil {
				t.Fatal(err)
			}
			lk.Type = syscall.F_UNLCK
			if err := syscall.FcntlFlock(f.Fd(), syscall.F_SETLK, &lk); err != nil {
				t.Fatal(err)
			}
			if err := <-done; err != nil {
				t.Fatalf("%s: %v\n%s", tc.args[0], err, out.String())
			}

			if !strings.Contains(out.String(), "no longer in the active file") {
				t.Errorf("%s did not report the removed group:\n%s", tc.args[0], out.String())
			}
			if _, err := os.Stat(data); err != nil {
				t.Errorf("%s removed the data file of the removed group: %v\n%s", tc.args[0], err, out.String())
			}
			if b, err := os.ReadFile(over); err != nil || !bytes.Equal(b, index) {
				t.Errorf("%s changed the over file of the removed group (%d bytes, was %d): %v\n%s",
					tc.args[0], len(b), len(index), err, out.String())
			}
		})
	}
}

// kpRecordOffset returns the offset of the live record of key in a KP database.
func kpRecordOffset(t *testing.T, path, key string) int64 {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	pos := 0
	for _, l := range strings.SplitAfter(string(b), "\n") {
		if _, rest, ok := strings.Cut(l, ":"); ok && l[0] == '+' && strings.HasPrefix(rest, key+" ") {
			return int64(pos)
		}
		pos += len(l)
	}
	t.Fatalf("no live record for %s in %s:\n%s", key, path, b)
	return 0
}

// waitLockWaiter returns once process pid is blocked waiting for a POSIX
// lock, as /proc/locks lists it ("-> POSIX ... pid ..."), or after a second
// where there is no /proc/locks.  It fails when the process exits first.
func waitLockWaiter(pid int, done chan error) error {
	if _, err := os.Stat("/proc/locks"); err != nil {
		select {
		case err := <-done:
			done <- err
			return fmt.Errorf("exited first (%v)", err)
		case <-time.After(time.Second):
			return nil
		}
	}
	want := strconv.Itoa(pid)
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		b, _ := os.ReadFile("/proc/locks")
		for _, l := range strings.Split(string(b), "\n") {
			fs := strings.Fields(l)
			if len(fs) > 5 && fs[1] == "->" && fs[2] == "POSIX" && fs[5] == want {
				return nil
			}
		}
		select {
		case err := <-done:
			done <- err
			return fmt.Errorf("exited first (%v)", err)
		case <-time.After(20 * time.Millisecond):
		}
	}
	return fmt.Errorf("not blocked on a lock after 30s")
}

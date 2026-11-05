package e2e

// lib/kpdb.c fixes of the kpdb-batch change:
//
//	D-90   KPDBSort() stored the offset of every record with its own pwrite.
//	D-117  a lookup that ran while a sort rewrote the offsets could report
//	       "not found" for a key that exists.
//	D-118  KPDBWrite() with a NULL token (documented as "unlock only") crashed.
//	D-122  a KP file past 2 GB wrapped the int offsets.
//
// The first three compile lib/kpdb.c unchanged with a small stand-in for
// defs.h and a driver, as TestKPDBLockRelookup does; the driver counts the
// pwrite() calls and can stop the sort at a fixed point, so no timing is
// involved.  The last one makes sparse files and runs dkp.
//
// Needs a C compiler and the source tree for the first three; skips
// otherwise, and fails instead when E2E_REQUIRE_SRC=1 is set.  E2E_SRC may
// point at another tree (for example a base checkout) to run them against.

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func kpbBuild(t *testing.T) string {
	t.Helper()
	cc := wmCC(t)
	root := wildmatSource(t)
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
	if err := os.WriteFile(filepath.Join(dir, "defs.h"), []byte(kpRelockDefs+kpbDefsExtra), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "driver.c"), []byte(kpbDriver), 0o644); err != nil {
		t.Fatal(err)
	}
	exe := filepath.Join(dir, "kpbatch")
	cmd := exec.Command(cc, "-O1", "-g", "-std=gnu99", "-D_GNU_SOURCE", "-D_FILE_OFFSET_BITS=64",
		"-o", exe, filepath.Join(dir, "kpdb.c"), filepath.Join(dir, "driver.c"))
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("compiling the kpdb driver: %v\n%s", err, out)
	}
	return exe
}

func kpbRun(t *testing.T, exe, mode string) map[string]bool {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	db := filepath.Join(t.TempDir(), "active.kp")
	out, err := exec.CommandContext(ctx, exe, mode, db).CombinedOutput()
	if err != nil {
		t.Fatalf("driver %s: %v\n%s", mode, err, out)
	}
	t.Logf("%s:\n%s", mode, out)
	lines := map[string]bool{}
	for _, l := range strings.Split(string(out), "\n") {
		lines[l] = true
	}
	return lines
}

func kpbWant(t *testing.T, lines map[string]bool, want ...string) {
	t.Helper()
	for _, w := range want {
		if !lines[w] {
			t.Errorf("missing %q", w)
		}
	}
}

// TestKPDBSortFewWrites (D-90): sorting 600 records takes a constant number
// of write calls (marker, timestamp, completion), not one per record, and
// the offsets are right.  Fails on main: 600 + 3 writes.
func TestKPDBSortFewWrites(t *testing.T) {
	exe := kpbBuild(t)
	lines := kpbRun(t, exe, "sortcount")
	kpbWant(t, lines, "records 600", "writes ok", "table ok", "all found")
}

// TestKPDBInterruptedSortResorts (D-90): a sort that stopped after its
// marker, and one that stopped with part of the offsets stored, are
// resorted by the next open.
func TestKPDBInterruptedSortResorts(t *testing.T) {
	exe := kpbBuild(t)
	lines := kpbRun(t, exe, "interrupt")
	kpbWant(t, lines,
		"killed after marker", "marker set", "table ok after resort of marker only",
		"half stored", "table ok after resort of half stored", "marker cleared")
}

// TestKPDBLookupDuringSort (D-117): a table with half of its offsets
// rewritten (what a reader sees in the middle of a sort) is held in exactly
// that state, by stopping the sorting process right after its marker write,
// while a second process looks up every key.  None may be reported missing.
// Fails on main: 114 of 600 keys are missed.
func TestKPDBLookupDuringSort(t *testing.T) {
	exe := kpbBuild(t)
	lines := kpbRun(t, exe, "window")
	kpbWant(t, lines, "sorter at marker", "P2 misses 0", "table ok")
}

// TestKPDBWriteNullToken (D-118): a NULL token only unlocks or looks up; it
// does not crash, write or create.
func TestKPDBWriteNullToken(t *testing.T) {
	exe := kpbBuild(t)
	lines := kpbRun(t, exe, "nulltok")
	kpbWant(t, lines, "unlock returns 0", "record unchanged", "lock released",
		"missing key returns -1", "missing key not created", "KP_LOCK keeps lock")
}

// TestKPDBTooLarge (D-122): a KP file at or past 2 GB is refused with a message and a
// non-zero exit, not wrapped into negative offsets.  Sparse files; skipped if the file
// system cannot make them.
func TestKPDBTooLarge(t *testing.T) {
	dkp := filepath.Join(bin(t), "dkp")
	run := func(args ...string) (string, error) {
		ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
		defer cancel()
		out, err := exec.CommandContext(ctx, dkp, args...).CombinedOutput()
		return string(out), err
	}
	mk := func(t *testing.T, size int64) string {
		t.Helper()
		db := filepath.Join(t.TempDir(), "kp.db")
		if out, err := run("-w", db, "aa", "x=1"); err != nil {
			t.Fatalf("dkp -w: %v\n%s", err, out)
		}
		f, err := os.OpenFile(db, os.O_RDWR, 0)
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		if err := f.Truncate(size); err != nil {
			t.Skipf("cannot make a sparse file of %d bytes: %v", size, err)
		}
		if _, err := f.WriteAt([]byte("\n"), size-1); err != nil {
			t.Skipf("cannot write into the sparse file: %v", err)
		}
		return db
	}
	t.Run("open", func(t *testing.T) {
		db := mk(t, 3<<30)
		out, err := run("-r", db, "aa")
		if err == nil || !strings.Contains(out, "too large") {
			t.Errorf("dkp -r on a 3 GB file: err=%v out=%q, want exit != 0 and 'too large'", err, out)
		}
	})
	t.Run("append", func(t *testing.T) {
		// about 15 s: the lookup before the append scans the 2 GB of zero bytes
		if testing.Short() {
			t.Skip("slow: scans a 2 GB sparse file")
		}
		const size = 1<<31 - 10
		db := mk(t, size)
		out, err := run("-w", db, "bb", "x=1")
		if err == nil || !strings.Contains(out, "too large") {
			t.Errorf("dkp -w that would pass 2 GB: err=%v out=%q, want exit != 0 and 'too large'", err, out)
		}
		if st, e := os.Stat(db); e != nil || st.Size() != size {
			t.Errorf("file after the refused append: %v %v, want size %d", st, e, int64(size))
		}
	})
}

const kpbDefsExtra = `
#include <sys/syscall.h>
#include <sys/types.h>
/* kpdb.c calls kp_test_pwrite() so the driver can count and stop writes */
ssize_t kp_test_pwrite(int fd, const void *buf, size_t len, off_t offset);
#define pwrite kp_test_pwrite
`

const kpbDriver = `#include "defs.h"
#include <signal.h>
#include <sys/wait.h>
#undef pwrite

KPDB *KPDBOpen(const char *fileName, int oflags);
void KPDBClose(KPDB *kpdb);
const char *KPDBReadRecord(KPDB *kpdb, const char *key, int lockMe, int *preclen);
const char *KPDBGetField(const char *rec, int recLen, const char *tok, int *plen, const char *def);
int KPDBWrite(KPDB *kpdb, const char *key, const char *tok, const char *data, int lockMe);
int KPDBReadRecordOff(KPDB *kpdb, const char *key, int lockMe, int *preclen);
int KPDBLookup(KPDB *kpdb, const char *key, int lockMe, int forceCheck);
void KPDBReSort(KPDB *kpdb);
int KPDBAppendCount(KPDB *kpdb);

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

static int AFd = -1, BFd = -1;		/* A: child -> parent, B: parent -> child */
static int IsChild;
static int StopAtMarker;		/* parent: stop after the sort's marker write */
static int KillAtMarker;		/* child: _exit after the marker write */
static long Writes;			/* pwrite calls */

static void
say(const char *fmt, ...)
{
    char buf[256];
    va_list va;
    int n;

    va_start(va, fmt);
    n = vsnprintf(buf, sizeof(buf), fmt, va);
    va_end(va);
    if (n > 0)
	(void)!write(1, buf, n < (int)sizeof(buf) ? n : (int)sizeof(buf) - 1);
}

ssize_t
kp_test_pwrite(int fd, const void *buf, size_t len, off_t offset)
{
    ssize_t r = pwrite(fd, buf, len, offset);
    ++Writes;
    if ((StopAtMarker || KillAtMarker) && offset == 26 && len == 8 &&
					memcmp(buf, "0FFFFFFF", 8) == 0) {
	if (KillAtMarker)
	    _exit(9);
	StopAtMarker = 0;
	say("sorter at marker\n");
	(void)!write(BFd, "G", 1);	/* P2: go */
	for (;;) {			/* until P2 is done or waits for the lock */
	    char c;
	    alarm(30);
	    if (read(AFd, &c, 1) != 1)
		_exit(4);
	    alarm(0);
	    if (c == 'W' || c == 'D')
		break;
	}
    }
    return r;
}

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
    if (IsChild && offset == 4 && (flags & 0x0F) == XLOCK_EX && !(flags & XLOCK_NB))
	(void)!write(AFd, "W", 1);	/* about to wait for the append lock */
    return fcntl(fd, (flags & XLOCK_NB) ? F_SETLK : F_SETLKW, &fl);
}

#define NKEYS 600
static char Keys[NKEYS][16];
static int Nkeys = NKEYS;

static void
makeKeys(void)
{
    int i;
    for (i = 0; i < NKEYS; ++i)
	snprintf(Keys[i], sizeof(Keys[i]), "grp.%05d", (i * 7919) % NKEYS);	/* unsorted order */
}

static KPDB *
build(const char *path)
{
    KPDB *db = KPDBOpen(path, O_RDWR|O_CREAT);
    int i;

    if (db == NULL)
	exit(2);
    db->kp_ResortLimit = 1000000;		/* no automatic sort while building */
    for (i = 0; i < Nkeys; ++i)
	KPDBWrite(db, Keys[i], "NX", "0000000000", 0);
    return db;
}

static char *
slurp(const char *path, long *len)
{
    FILE *f = fopen(path, "rb");
    char *b;
    long n;

    if (f == NULL) exit(2);
    fseek(f, 0, SEEK_END);
    n = ftell(f);
    rewind(f);
    b = malloc(n + 1);
    if (fread(b, 1, n, f) != (size_t)n) exit(2);
    b[n] = 0;
    fclose(f);
    *len = n;
    return b;
}

/* read the whole file through an open descriptor (never close() another
 * descriptor of a file whose locks we hold: that drops them all) */
static char *
slurpFd(int fd, long *len)
{
    struct stat st;
    char *b;

    if (fstat(fd, &st) < 0) exit(2);
    b = malloc(st.st_size + 1);
    if (pread(fd, b, st.st_size, 0) != st.st_size) exit(2);
    b[st.st_size] = 0;
    *len = st.st_size;
    return b;
}

static int
keycmp(const void *a, const void *b) { return strcmp((const char *)a, (const char *)b); }

/*
 * The offset in the k-th record line (file order) must be the line of the
 * k-th key in sorted order.  Returns the number of wrong lines.
 */
static int
checkTable(const char *path)
{
    long len, pos, hl = 0;
    char *b = slurp(path, &len);
    char sorted[NKEYS][16];
    int n = 0, k, bad = 0;

    memcpy(sorted, Keys, sizeof(sorted));
    qsort(sorted, NKEYS, 16, keycmp);
    while (hl < len && b[hl] != '\n') ++hl;
    for (pos = hl + 1, k = 0; pos < len && k < NKEYS; ++k) {
	char *nl = memchr(b + pos, '\n', len - pos);
	long off;
	char key[16];
	int i;

	if (nl == NULL) break;
	off = strtol(b + pos + 1, NULL, 16);
	if (off <= hl || off >= len) { ++bad; pos = nl - b + 1; ++n; continue; }
	for (i = 0; i < 15 && b[off + 15 + i] != ' ' && b[off + 15 + i] != '\n'; ++i)
	    key[i] = b[off + 15 + i];
	key[i] = 0;
	if (strcmp(key, sorted[k]) != 0) ++bad;
	pos = nl - b + 1;
	++n;
    }
    free(b);
    return (n == NKEYS) ? bad : bad + 1000;
}

static int
markerIs(const char *path, const char *want)
{
    long len;
    char *b = slurp(path, &len);
    int r = (len > 34 && memcmp(b + 26, want, 8) == 0);
    free(b);
    return r;
}

/* store the offset of the records of the first half in the wrong order */
static void
corruptHalf(const char *path)
{
    long len, pos, hl = 0;
    char *b = slurp(path, &len);
    int fd = open(path, O_RDWR), k = 0;
    char first[9] = "";

    while (hl < len && b[hl] != '\n') ++hl;
    for (pos = hl + 1; pos < len && k < NKEYS / 2; ++k) {
	char *nl = memchr(b + pos, '\n', len - pos);
	char *nx = nl + 1;
	if (k == 0) memcpy(first, b + pos + 1, 8);
	/* rotate: line k gets the offset of line k+1 */
	if (pwrite(fd, nx + 1, 8, pos + 1) != 8) exit(2);
	pos = nl - b + 1;
    }
    (void)first;
    if (pwrite(fd, "0FFFFFFF", 8, 26) != 8) exit(2);
    close(fd);
    free(b);
}

static int
allFound(KPDB *db)
{
    int i, miss = 0;
    for (i = 0; i < Nkeys; ++i)
	if (KPDBLookup(db, Keys[i], 0, 0) != 0) ++miss;
    return miss;
}

static int
modeSortcount(const char *path)
{
    KPDB *db = build(path);
    long before;

    KPDBReSort(db);			/* sorted once */
    say("records %d\n", Nkeys);
    corruptHalf(path);			/* offsets wrong again, sort once more */
    before = Writes;
    KPDBReSort(db);
    say("sort used %ld writes\n", Writes - before);
    if (Writes - before <= 5)
	say("writes ok\n");
    say("%s\n", checkTable(path) == 0 ? "table ok" : "table WRONG");
    KPDBClose(db);
    db = KPDBOpen(path, O_RDWR|O_CREAT);
    say("%s\n", allFound(db) == 0 ? "all found" : "some NOT found");
    return 0;
}

static int
modeInterrupt(const char *path)
{
    KPDB *db = build(path);
    pid_t p;
    int st;

    KPDBReSort(db);
    KPDBClose(db);

    /* sorter killed right after its marker write */
    if ((p = fork()) == 0) {
	KPDB *d = KPDBOpen(path, O_RDWR|O_CREAT);
	KillAtMarker = 1;
	KPDBReSort(d);
	_exit(0);
    }
    waitpid(p, &st, 0);
    if (WIFEXITED(st) && WEXITSTATUS(st) == 9)
	say("killed after marker\n");
    if (markerIs(path, "0FFFFFFF"))
	say("marker set\n");
    db = KPDBOpen(path, O_RDWR|O_CREAT);			/* first opener: resorts */
    say("%s\n", checkTable(path) == 0 ? "table ok after resort of marker only" : "table WRONG after resort of marker only");
    KPDBClose(db);

    /* part of the offsets stored, marker still set */
    corruptHalf(path);
    say("half stored\n");
    db = KPDBOpen(path, O_RDWR|O_CREAT);
    say("%s\n", checkTable(path) == 0 ? "table ok after resort of half stored" : "table WRONG after resort of half stored");
    if (markerIs(path, "00000000"))
	say("marker cleared\n");
    KPDBClose(db);
    return 0;
}

/*
 * What a reader sees in the middle of a sort: the first 300 keys were
 * sorted in an earlier pass, 300 more were appended since (offset 0), and
 * the running sort has stored its new offsets for the first M lines only.
 * The new offsets are taken from a sorted copy of the file.
 */
#define M 450
static KPDB *
midSort(const char *path)
{
    KPDB *db = KPDBOpen(path, O_RDWR|O_CREAT);
    char copy[512];
    long len, pos, hl = 0, clen;
    char *b, *cb;
    int i, k, fd;
    KPDB *c;

    db->kp_ResortLimit = 1000000;
    for (i = 0; i < 300; ++i)
	KPDBWrite(db, Keys[i], "NX", "0000000000", 0);
    KPDBReSort(db);
    for (i = 300; i < Nkeys; ++i)
	KPDBWrite(db, Keys[i], "NX", "0000000000", 0);
    snprintf(copy, sizeof(copy), "%s.sorted", path);
    b = slurpFd(db->kp_Fd, &len);
    fd = open(copy, O_RDWR|O_CREAT|O_TRUNC, 0644);
    if (write(fd, b, len) != len) exit(2);
    close(fd);
    c = KPDBOpen(copy, O_RDWR);
    KPDBReSort(c);
    KPDBClose(c);
    cb = slurp(copy, &clen);
    fd = db->kp_Fd;		/* not closed: its locks are what keeps P2 from sorting */
    while (hl < len && b[hl] != '\n') ++hl;
    for (pos = hl + 1, k = 0; pos < len && k < M; ++k) {
	char *nl = memchr(b + pos, '\n', len - pos);
	if (pwrite(fd, cb + pos + 1, 8, pos + 1) != 8) exit(2);
	pos = nl - b + 1;
    }
    if (pwrite(fd, "0FFFFFFF", 8, 26) != 8) exit(2);
    free(b);
    free(cb);
    return db;
}

static int
modeWindow(const char *path)
{
    KPDB *db = midSort(path);
    int a[2], b[2];
    pid_t p;
    char c;
    int st;

    if (pipe(a) || pipe(b)) exit(2);
    AFd = a[1]; BFd = b[1];
    if ((p = fork()) == 0) {
	int miss;
	KPDB *d = KPDBOpen(path, O_RDWR);	/* second opener: no sort */
	IsChild = 1;
	AFd = a[1];
	/* no remap on a miss: that would take the append lock too (KPDBReOpen) */
	d->kp_AppendSeq = KPDBAppendCount(d);
	(void)!write(a[1], "R", 1);
	if (read(b[0], &c, 1) != 1) _exit(5);
	miss = allFound(d);
	say("P2 misses %d\n", miss);
	(void)!write(a[1], "D", 1);
	_exit(0);
    }
    AFd = a[0]; BFd = b[1];
    if (read(a[0], &c, 1) != 1) exit(5);	/* P2 has opened the file */
    StopAtMarker = 1;
    KPDBReSort(db);
    waitpid(p, &st, 0);
    say("%s\n", checkTable(path) == 0 ? "table ok" : "table WRONG");
    return 0;
}

static int
modeNulltok(const char *path)
{
    KPDB *db = KPDBOpen(path, O_RDWR|O_CREAT);
    int len, off, st;
    const char *rec, *f;
    pid_t p;

    KPDBWrite(db, "k1", "NX", "0000000007", 0);
    off = KPDBReadRecordOff(db, "k1", KP_LOCK, &len);	/* locked */
    if (KPDBWrite(db, "k1", NULL, "x", KP_UNLOCK) == 0)
	say("unlock returns 0\n");
    rec = KPDBReadRecord(db, "k1", 0, &len);
    f = rec ? KPDBGetField(rec, len, "NX", &len, NULL) : NULL;
    if (f && memcmp(f, "0000000007", 10) == 0)
	say("record unchanged\n");
    if ((p = fork()) == 0)
	_exit(hflock(open(path, O_RDWR), off, XLOCK_EX | XLOCK_NB) == 0 ? 0 : 1);
    waitpid(p, &st, 0);
    if (WIFEXITED(st) && WEXITSTATUS(st) == 0)
	say("lock released\n");
    if (KPDBWrite(db, "nokey", NULL, "x", 0) == -1)
	say("missing key returns -1\n");
    if (KPDBLookup(db, "nokey", 0, 1) != 0)
	say("missing key not created\n");
    off = KPDBReadRecordOff(db, "k1", 0, &len);
    if (KPDBWrite(db, "k1", NULL, "x", KP_LOCK) == 0) {
	if ((p = fork()) == 0)
	    _exit(hflock(open(path, O_RDWR), off, XLOCK_EX | XLOCK_NB) == 0 ? 0 : 1);
	waitpid(p, &st, 0);
	/* the parent holds it: locks of a process do not exclude itself, the child must fail */
	if (WIFEXITED(st) && WEXITSTATUS(st) == 1)
	    say("KP_LOCK keeps lock\n");
    }
    return 0;
}

int
main(int ac, char **av)
{
    if (ac != 3)
	return 2;
    makeKeys();
    if (strcmp(av[1], "sortcount") == 0) return modeSortcount(av[2]);
    if (strcmp(av[1], "interrupt") == 0) return modeInterrupt(av[2]);
    if (strcmp(av[1], "window") == 0) return modeWindow(av[2]);
    if (strcmp(av[1], "nulltok") == 0) return modeNulltok(av[2]);
    return 2;
}
`

package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// shortWriteSource makes the first write() to a file whose path holds SHORTWRITE_FILE a
// short one (half of the bytes), and every ftruncate() of such a file fail with EIO.
const shortWriteSource = `#define _GNU_SOURCE
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
static int
mine(int fd)
{
    char path[4096];
    const char *only = getenv("SHORTWRITE_FILE");

    path[0] = 0;
    fdpath(fd, path, sizeof(path));
    return (only != NULL && path[0] != 0 && strstr(path, only) != NULL);
}

ssize_t
write(int fd, const void *buf, size_t n)
{
    static ssize_t (*real)(int, const void *, size_t);
    static int done;

    if (real == NULL)
	real = (ssize_t (*)(int, const void *, size_t))dlsym(RTLD_NEXT, "write");
    if (!done && n > 1 && mine(fd)) {
	size_t k = n / 2;

	/* always cut inside a line */
	if (k > 1 && ((const char *)buf)[k - 1] == '\n')
	    --k;
	done = 1;
	return real(fd, buf, k);
    }
    return real(fd, buf, n);
}

int
ftruncate(int fd, off_t len)
{
    static int (*real)(int, off_t);

    if (real == NULL)
	real = (int (*)(int, off_t))dlsym(RTLD_NEXT, "ftruncate");
    if (mine(fd)) {
	errno = EIO;
	return -1;
    }
    return real(fd, len);
}

#ifdef __linux__
/* glibc has it as a symbol of its own; elsewhere it is ftruncate itself */
int
ftruncate64(int fd, off_t len)
{
    return ftruncate(fd, len);
}
#endif
`

// TestDeepRevQueueShortWriteNotRepeated: when a write to a queue file is short, diablo cuts
// the written part off again and keeps the buffer for the next flush. When that cut fails
// too, the written part stays in the file; the next flush wrote the whole buffer behind it,
// so the queue got a cut line glued to the first line, and lines twice. Only the rest must
// be written.
//
// The fault injector makes the first write to the queue file short and its ftruncate fail.
func TestDeepRevQueueShortWriteNotRepeated(t *testing.T) {
	so := preloadShim(t, "shortwrite", shortWriteSource)
	s := NewSite(t, SiteOptions{Newsfeeds: OutNewsfeeds,
		Env: []string{lockFaultEnv(so), "SHORTWRITE_FILE=/dqueue/" + OutLabel}})
	ids := s.Inject(t, "e2e.test", 4)

	limit := time.Now().Add(10 * time.Second)
	for !strings.Contains(generalLog(s), "queue file truncate failed") {
		if time.Now().After(limit) {
			t.Fatalf("diablo did not log a failed truncate of dqueue/%s; the fault injector did not act", OutLabel)
		}
		s.Dicmd(t, "flush")
		time.Sleep(100 * time.Millisecond)
	}
	s.Dicmd(t, "flush")
	q := deepRevWaitQueue(t, s, OutLabel, len(ids), 5*time.Second)
	if missing := deepRevMissing(q, ids); len(missing) != 0 {
		t.Errorf("%d of %d accepted articles are not in the queue: %q", len(missing), len(ids), missing)
	}
	raw := s.ReadFile(t, filepath.Join("dqueue", OutLabel))
	for _, id := range ids {
		if n := strings.Count(raw, id); n != 1 {
			t.Errorf("%s is in dqueue/%s %d times, want 1", id, OutLabel, n)
		}
	}
	for _, l := range strings.Split(strings.TrimSuffix(raw, "\n"), "\n") {
		if strings.Count(l, "<") != 1 {
			t.Errorf("not one queue entry per line: %q", l)
		}
	}
	if t.Failed() {
		t.Logf("dqueue/%s:\n%s", OutLabel, raw)
	}
}

// TestDeepRevQueueShortWriteAcrossRotation: dspoolout renames the queue file and then asks
// diablo for a flush. When the file ends in the first part of a line (short write, failed
// cut) and diablo opens the queue file by name for the retry, the rest of the line goes to
// a new file: neither file has the whole entry. The rest must go to the file that has the
// first part.
func TestDeepRevQueueShortWriteAcrossRotation(t *testing.T) {
	so := preloadShim(t, "shortwrite", shortWriteSource)
	s := NewSite(t, SiteOptions{Newsfeeds: OutNewsfeeds,
		Env: []string{lockFaultEnv(so), "SHORTWRITE_FILE=/dqueue/" + OutLabel}})
	ids := s.Inject(t, "e2e.test", 4)

	limit := time.Now().Add(10 * time.Second)
	for !strings.Contains(generalLog(s), "queue file truncate failed") {
		if time.Now().After(limit) {
			t.Fatalf("diablo did not log a failed truncate of dqueue/%s; the fault injector did not act", OutLabel)
		}
		s.Dicmd(t, "flush")
		time.Sleep(100 * time.Millisecond)
	}
	live := filepath.Join(s.Dir, "dqueue", OutLabel)
	rotated := live + ".S00001"
	if err := os.Rename(live, rotated); err != nil {
		t.Fatal(err)
	}
	if b, err := os.ReadFile(rotated); err != nil || len(b) == 0 || b[len(b)-1] == '\n' {
		t.Fatalf("the queue file does not end inside a line before the retry (%v): %q", err, b)
	}

	var raw string
	limit = time.Now().Add(10 * time.Second)
	for {
		s.Dicmd(t, "flush")
		raw = ""
		for _, f := range []string{rotated, live} {
			b, err := os.ReadFile(f)
			if err != nil && !os.IsNotExist(err) {
				t.Fatal(err)
			}
			if len(b) != 0 && b[len(b)-1] != '\n' {
				b = append(b, "<cut>\n"...)
			}
			raw += string(b)
		}
		n := 0
		for _, id := range ids {
			n += strings.Count(raw, id)
		}
		if n >= len(ids) || time.Now().After(limit) {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	q := strings.FieldsFunc(raw, func(r rune) bool { return r == '\n' })
	if missing := deepRevMissing(q, ids); len(missing) != 0 {
		t.Errorf("%d of %d accepted articles have no whole queue entry: %q", len(missing), len(ids), missing)
	}
	if len(q) != len(ids) {
		t.Errorf("%d queue lines in the two files, want %d", len(q), len(ids))
	}
	for _, l := range q {
		if f := strings.Fields(l); len(f) < 3 || strings.Count(l, "<") != 1 || strings.Contains(l, "<cut>") {
			t.Errorf("not a whole queue entry: %q", l)
		}
	}
	if t.Failed() {
		for _, f := range []string{rotated, live} {
			b, _ := os.ReadFile(f)
			t.Logf("%s:\n%s", filepath.Base(f), b)
		}
	}
}

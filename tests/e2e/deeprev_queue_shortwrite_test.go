package e2e

import (
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
	done = 1;
	return real(fd, buf, n / 2);
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

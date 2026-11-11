package e2e

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/go-while/nntp-testkit/fakeserver"
)

// backCopySource is a memcpy() that copies from the last byte to the first, as a C library may
// (glibc has done it on some processors), and a memmove() that is correct. Each writes a line
// to the file MEMCPY_OVERLAP_FILE when source and destination overlap and the copy is longer
// than 8000 bytes (only the compaction of dnewslink's response buffer is that long here):
// "memcpy" or "memmove". The memmove line shows that the overlapping move happened and that
// the library sees these calls, so a test that sees no memcpy line has not just missed it.
const backCopySource = `#define _GNU_SOURCE
#include <fcntl.h>
#include <stddef.h>
#include <stdint.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>

static int
overlap(const void *d, const void *s, size_t n)
{
    uintptr_t a = (uintptr_t)d, b = (uintptr_t)s;

    return n > 8000 && a < b + n && b < a + n;
}

static void
note(const char *what)
{
    const char *f = getenv("MEMCPY_OVERLAP_FILE");
    int fd;

    if (f != NULL && (fd = open(f, O_WRONLY|O_CREAT|O_APPEND, 0644)) >= 0) {
	(void)write(fd, what, strlen(what));
	(void)write(fd, "\n", 1);
	close(fd);
    }
}

static void *
backcopy(void *d, const void *s, size_t n)
{
    unsigned char *dp = d;
    const unsigned char *sp = s;

    if (overlap(d, s, n))
	note("memcpy");
    while (n-- > 0)
	dp[n] = sp[n];
    return d;
}

static void *
move(void *d, const void *s, size_t n)
{
    unsigned char *dp = d;
    const unsigned char *sp = s;
    size_t i;

    if (overlap(d, s, n))
	note("memmove");
    if ((uintptr_t)dp < (uintptr_t)sp) {
	for (i = 0; i < n; ++i)
	    dp[i] = sp[i];
    } else {
	while (n-- > 0)
	    dp[n] = sp[n];
    }
    return d;
}

void *
memcpy(void *d, const void *s, size_t n)
{
    return backcopy(d, s, n);
}

void *
__memcpy_chk(void *d, const void *s, size_t n, size_t dlen)
{
    if (n > dlen)
	abort();
    return backcopy(d, s, n);
}

void *
memmove(void *d, const void *s, size_t n)
{
    return move(d, s, n);
}

void *
__memmove_chk(void *d, const void *s, size_t n, size_t dlen)
{
    if (n > dlen)
	abort();
    return move(d, s, n);
}
`

// TestDeepRevDnewslinkResponseMemmove: when dnewslink's response buffer is full and a line
// is not complete yet, it moves the rest of the buffer to its start. Source and destination
// overlap when that rest is longer than what was used up; memcpy() is not defined for that.
// With a C library that copies from the end, the start of the response is overwritten:
// here the "203" of the reply to MODE STREAM, and dnewslink goes on without streaming.
//
// A greeting of 6000 bytes uses up the first part of the 16 KB buffer, then a reply to MODE
// STREAM of 12000 bytes fills it before its end of line.
func TestDeepRevDnewslinkResponseMemmove(t *testing.T) {
	so := preloadShim(t, "backcopy", backCopySource)
	mark := filepath.Join(t.TempDir(), "overlap")
	s := NewSite(t, SiteOptions{NoStart: true})
	s.Env = append(s.Env, lockFaultEnv(so), "MEMCPY_OVERLAP_FILE="+mark)
	dnlSpool(t, s)
	var stream atomic.Bool
	srv := fakeserver.New(t, fakeserver.Options{Greeting: "200 " + strings.Repeat("g", 6000),
		Handler: fakeserver.LineHandler(func(_ *fakeserver.Conn, line string) ([]string, bool) {
			l := strings.ToLower(strings.TrimSpace(line))
			switch {
			case strings.HasPrefix(l, "mode stream"):
				return []string{"203 " + strings.Repeat("s", 12000)}, false
			case strings.HasPrefix(l, "check "):
				stream.Store(true)
				return []string{"438 " + strings.Fields(line)[1]}, false
			case strings.HasPrefix(l, "ihave "):
				return []string{"435 " + strings.Fields(line)[1]}, false
			case strings.HasPrefix(l, "quit"):
				return []string{"205 bye"}, true
			}
			return nil, false
		})})
	if err := os.WriteFile(filepath.Join(s.Dir, "dqueue", "out.S00000"),
		[]byte("D.00000001/B.0001 <m@e2e.test> 0,100\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := s.TryRun("dnewslink", "-N", "1", "-S", "0", "-b", "out.S%05d", "-h", "127.0.0.1",
		"-P", strconv.Itoa(srv.Port()), "-t", "5")
	t.Logf("dnewslink: err=%v\n%s", err, out)
	b, _ := os.ReadFile(mark)
	notes := string(b)
	if n := strings.Count(notes, "memcpy\n"); n != 0 {
		t.Errorf("dnewslink copied overlapping memory with memcpy (%d times)", n)
	} else if !strings.Contains(notes, "memmove\n") {
		t.Fatalf("the scenario did not develop: no overlapping move of the response buffer was seen (notes: %q)", notes)
	}
	if !stream.Load() {
		t.Errorf("dnewslink did not stream: the reply to MODE STREAM was damaged in the buffer")
	}
}

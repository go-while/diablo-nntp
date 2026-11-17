package e2e

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// readFailSource is an LD_PRELOAD shim: read() of a descriptor whose path ends in
// READFAIL_PATH fails with EIO on its READFAIL_CALL-th call (counted over those descriptors
// only) and every call after it, and appends a line to the file READFAIL_MARK when it does.
// __read_chk is read() as a build with _FORTIFY_SOURCE calls it for a buffer of known size.
const readFailSource = `#define _GNU_SOURCE
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
fail(int fd)
{
    static int calls;
    const char *suffix = getenv("READFAIL_PATH");
    const char *at = getenv("READFAIL_CALL");
    const char *mark = getenv("READFAIL_MARK");
    char path[4096];
    size_t n, m;
    FILE *f;

    if (suffix == NULL || at == NULL)
	return 0;
    fdpath(fd, path, sizeof(path));
    n = strlen(path);
    m = strlen(suffix);
    if (n < m || strcmp(path + n - m, suffix) != 0)
	return 0;
    if (++calls < atoi(at))
	return 0;
    if (mark != NULL && (f = fopen(mark, "a")) != NULL) {
	fprintf(f, "read %d of %s\n", calls, path);
	fclose(f);
    }
    errno = EIO;
    return 1;
}

ssize_t
read(int fd, void *buf, size_t len)
{
    static ssize_t (*real)(int, void *, size_t);

    if (real == NULL)
	real = (ssize_t (*)(int, void *, size_t))dlsym(RTLD_NEXT, "read");
    if (fail(fd))
	return -1;
    return real(fd, buf, len);
}

#ifdef __GLIBC__
ssize_t
__read_chk(int fd, void *buf, size_t len, size_t buflen)
{
    (void)buflen;
    return read(fd, buf, len);
}
#endif
`

// TestDidumpReadError (D-287): when read() of the history failed in the
// middle of a dump, didump printed "Error reading file" on stderr but still ended the dump
// with the "." line and exit status 0, so a didump | diload could take a cut dump for the
// whole history. Now the dump has no "." line after a read error and didump
// exits 1.
//
// A preloaded library makes the third read() of dhistory fail with EIO: the header and the
// first block of records are read, so entries are printed before the error. A history cut
// short (a truncated file) does not reach this path: read() returns the last whole block and
// then 0, and didump dumps the whole records there are. The same history dumped without the
// fault ends with "." and exit status 0. Fails on main: "." and exit status 0 after the error.
func TestDidumpReadError(t *testing.T) {
	so := preloadShim(t, "readfail", readFailSource)
	s := NewSite(t, SiteOptions{})
	s.Inject(t, "e2e.test", 3)
	hist := filepath.Join(s.Dir, "dhistory")

	didump := func(env ...string) (stdout, stderr string, err error) {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		cmd := exec.CommandContext(ctx, filepath.Join(s.Bin, "didump"), hist)
		cmd.Dir = s.Dir
		cmd.Env = append(append(os.Environ(), s.Env...), env...)
		var out, errOut bytes.Buffer
		cmd.Stdout, cmd.Stderr = &out, &errOut
		err = cmd.Run()
		return out.String(), errOut.String(), err
	}
	lastLine := func(s string) string {
		lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
		return lines[len(lines)-1]
	}
	dumpLines := func(s string) (n int) {
		for _, line := range strings.Split(s, "\n") {
			if strings.HasPrefix(line, "DUMP ") {
				n++
			}
		}
		return n
	}

	// without the fault: the whole dump and its end line
	out, errOut, err := didump()
	if err != nil || lastLine(out) != "." || dumpLines(out) != 3 {
		t.Fatalf("didump of the good history: %v, want 3 DUMP lines and \".\" last\n%s\n%s", err, out, errOut)
	}

	mark := filepath.Join(t.TempDir(), "readfail")
	out, errOut, err = didump(lockFaultEnv(so), "READFAIL_PATH=/dhistory", "READFAIL_CALL=3", "READFAIL_MARK="+mark)
	if b, rerr := os.ReadFile(mark); rerr != nil || len(b) == 0 {
		t.Fatalf("the read fault was never hit (%v):\n%s\n%s", rerr, out, errOut)
	}
	if !strings.Contains(errOut, "Error reading file") {
		t.Errorf("didump did not report the read error:\n%s", errOut)
	}
	if dumpLines(out) == 0 {
		t.Errorf("no entries dumped before the read error (the fault came too early):\n%s\n%s", out, errOut)
	}
	for _, line := range strings.Split(out, "\n") {
		if line == "." {
			t.Errorf("didump ended a cut dump with the \".\" line:\n%s", out)
			break
		}
	}
	var ee *exec.ExitError
	if !errors.As(err, &ee) || ee.ExitCode() != 1 {
		t.Errorf("didump after a read error: %v, want exit status 1\n%s", err, errOut)
	}
}

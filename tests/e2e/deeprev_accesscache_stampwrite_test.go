package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// stampWriteFailSource makes write() to a file whose name ends in "/.accesslock" fail with
// ENOSPC while the file STAMPFAIL_FLAG exists, and marks each refused write.
const stampWriteFailSource = `#define _GNU_SOURCE
#define __EXTENSIONS__
#include <dlfcn.h>
#include <errno.h>
#include <fcntl.h>
#include <stdio.h>
#include <string.h>
#include <sys/types.h>
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
	    shim_mark("refused write to %s", path);
	    errno = ENOSPC;
	    return -1;
	}
    }
    return real(fd, buf, n);
}
`

// TestDeepRevAccessCacheStampWriteFails: the install of the access cache announces the new
// set by writing the lock file's stamp. That write was not checked: when it failed (a full
// disk) the install counted as done and was never tried again, while the lock file stayed
// empty and the readers (rightly) loaded nothing, so every client got 502 until dreader.access
// was changed or dreaderd restarted.
//
// A fresh site starts while a preloaded write() refuses writes to .accesslock (ENOSPC): no
// client may be served from an unannounced set. Once the fault is gone, the next try must
// install it: a client is served within 10 s.
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
	if strings.HasPrefix(g, "20") {
		t.Fatalf("with the stamp unwritten a client was served (%q): an unannounced access cache was loaded", strings.TrimSpace(g))
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

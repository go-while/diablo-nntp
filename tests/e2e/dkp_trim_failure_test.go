package e2e

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestDKPTrimMissingFile(t *testing.T) {
	dkp := filepath.Join(bin(t), "dkp")
	db := filepath.Join(t.TempDir(), "missing.kp")
	if out, err := exec.Command(dkp, "-t", db).CombinedOutput(); err == nil {
		t.Errorf("dkp reported success for a missing database: %s", out)
	}
}

// An installation failure must leave the original database untouched and be
// reported, rather than silently sorting the old file and returning success.
func TestDKPTrimRenameFailure(t *testing.T) {
	testDKPTrimFailure(t, `#include <errno.h>
int rename(const char *a, const char *b) {
    (void)a; (void)b; errno = EIO; return -1;
}
`)
}

// Fail a read after the first buffered chunk. The trim must not install the
// partial copy even when its temporary-file writes all succeeded.
func TestDKPTrimReadFailure(t *testing.T) {
	testDKPTrimFailure(t, `#define _GNU_SOURCE
#include <dlfcn.h>
#include <stdio.h>
#include <unistd.h>
char *fgets(char *s, int n, FILE *stream) {
    static char *(*real_fgets)(char *, int, FILE *);
    static int first = 1;
    char *r;
    if (!real_fgets) real_fgets = dlsym(RTLD_NEXT, "fgets");
    r = real_fgets(s, n, stream);
    if (first && r) { first = 0; close(fileno(stream)); }
    return r;
}
`)
}

func TestDKPTrimCloseFailure(t *testing.T) {
	if _, err := os.Stat("/proc/self/fd"); err != nil {
		t.Skip("the close fault injector needs /proc/self/fd")
	}
	testDKPTrimFailure(t, `#define _GNU_SOURCE
#include <dlfcn.h>
#include <errno.h>
#include <stdio.h>
#include <string.h>
#include <unistd.h>
int fclose(FILE *stream) {
    static int (*real_fclose)(FILE *);
    char link[64], path[4096];
    ssize_t n;
    int fail, r;
    if (!real_fclose) real_fclose = dlsym(RTLD_NEXT, "fclose");
    snprintf(link, sizeof(link), "/proc/self/fd/%d", fileno(stream));
    n = readlink(link, path, sizeof(path)-1);
    if (n >= 0) path[n] = 0; else path[0] = 0;
    fail = strstr(path, "active.kp.tmp") != NULL;
    r = real_fclose(stream);
    if (fail) { errno = EIO; return EOF; }
    return r;
}
`)
}

func testDKPTrimFailure(t *testing.T, source string) {
	t.Helper()
	if runtime.GOOS != "linux" {
		t.Skip("the local fault injector uses LD_PRELOAD on Linux")
	}
	cc := wmCC(t)
	dir := t.TempDir()
	c := filepath.Join(dir, "failrename.c")
	so := filepath.Join(dir, "failrename.so")
	if err := os.WriteFile(c, []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command(cc, "-shared", "-fPIC", "-o", so, c, "-ldl").CombinedOutput(); err != nil {
		t.Fatalf("build fault injector: %v\n%s", err, out)
	}
	dkp := filepath.Join(bin(t), "dkp")
	db := filepath.Join(dir, "active.kp")
	if out, err := exec.Command(dkp, "-w", db, "kept", "x="+strings.Repeat("v", 20000)).CombinedOutput(); err != nil {
		t.Fatalf("create database: %v\n%s", err, out)
	}
	before, err := os.ReadFile(db)
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(dkp, "-t", db)
	cmd.Env = append(os.Environ(), "LD_PRELOAD="+so)
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Errorf("dkp reported success after injected failure: %s", out)
	}
	after, err := os.ReadFile(db)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Error("original database changed after failed installation")
	}
	leftovers, err := filepath.Glob(db + ".tmp*")
	if err != nil || len(leftovers) != 0 {
		t.Errorf("temporary files left after failure: %q (%v)", leftovers, err)
	}
}

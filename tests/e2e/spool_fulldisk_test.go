package e2e

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestFeederFullDiskTryAgain: when no spool file can be created because the disk is full
// (ENOSPC from open() or mkdir(), as on a full ZFS pool), the article is not stored and not
// put in history, and the peer is told to try again later: 436 to IHAVE and 431 to TAKETHIS.
// Before, both got a reject (437 / 439, "IOError/Missing-dir") and the peer dropped the
// article. Once there is room again, the same articles are taken.
//
// A preloaded shim fails the creation of spool files (B.*) and directories (D.*) with ENOSPC
// while the file FULLDISK_FLAG exists.
func TestFeederFullDiskTryAgain(t *testing.T) {
	so := preloadShim(t, "fulldisk", fullDiskSource)
	flag := filepath.Join(t.TempDir(), "full")
	s := NewSite(t, SiteOptions{
		Config: map[string]string{"path_generallog": "%s/general.log"},
		Setup: func(tb testing.TB, s *Site) {
			s.Env = append(s.Env, lockFaultEnv(so), "FULLDISK_FLAG="+flag)
		},
	})
	p := s.Peer(t)
	if err := os.WriteFile(flag, nil, 0o644); err != nil {
		t.Fatal(err)
	}

	ihave := msgID(t)
	art := newArticle(ihave, "e2e.test", "full disk ihave", "body")
	expect(t, p, 335, "IHAVE %s", ihave)
	send(t, p, wire(art))
	status(t, p, 436, "IHAVE on a full disk")

	expect(t, p, 203, "MODE STREAM")
	take := msgID(t)
	takeArt := wire(newArticle(take, "e2e.test", "full disk takethis", "body"))
	if err := p.Send(fmt.Sprintf("TAKETHIS %s\r\n%s", take, takeArt)); err != nil {
		t.Fatal(err)
	}
	status(t, p, 431, "TAKETHIS on a full disk")

	logged, _ := os.ReadFile(filepath.Join(s.Dir, "log", "general.log"))
	if !strings.Contains(string(logged)+s.Proc.Stderr(), "No space left on device") {
		t.Errorf("no log line about the full disk in log/general.log or stderr:\n%s", logged)
	}

	// Room again: the same articles are taken (they are not in history).
	if err := os.Remove(flag); err != nil {
		t.Fatal(err)
	}
	expect(t, p, 335, "IHAVE %s", ihave)
	send(t, p, wire(art))
	status(t, p, 235, "IHAVE after room was made")
	if err := p.Send(fmt.Sprintf("TAKETHIS %s\r\n%s", take, takeArt)); err != nil {
		t.Fatal(err)
	}
	status(t, p, 239, "TAKETHIS after room was made")
	expect(t, p, 223, "STAT %s", ihave)
	expect(t, p, 223, "STAT %s", take)
	expect(t, p, 205, "QUIT")
}

const fullDiskSource = `#define _GNU_SOURCE
#define __EXTENSIONS__
#include <dlfcn.h>
#include <errno.h>
#include <fcntl.h>
#include <stdarg.h>
#include <stdlib.h>
#include <string.h>
#include <sys/stat.h>
#include <sys/types.h>
#include <unistd.h>

/* the disk is "full" for spool files and directories (prefix B. or D.) while the flag exists */
static int
full(const char *path, const char *prefix)
{
    const char *flag = getenv("FULLDISK_FLAG");
    const char *base = strrchr(path, '/');

    base = base ? base + 1 : path;
    return(flag && strncmp(base, prefix, 2) == 0 && access(flag, F_OK) == 0);
}

static int
fullopen(const char *name, const char *path, int flags, mode_t mode)
{
    int (*real)(const char *, int, ...);

    if ((flags & O_CREAT) && full(path, "B.")) {
	errno = ENOSPC;
	return(-1);
    }
    real = (int (*)(const char *, int, ...))dlsym(RTLD_NEXT, name);
    return(real(path, flags, mode));
}

int
open(const char *path, int flags, ...)
{
    mode_t mode = 0;

    if (flags & O_CREAT) {
	va_list ap;

	va_start(ap, flags);
	mode = (mode_t)va_arg(ap, int);
	va_end(ap);
    }
    return(fullopen("open", path, flags, mode));
}

#ifdef __linux__
/* glibc's open() with _FILE_OFFSET_BITS=64 */
int
open64(const char *path, int flags, ...)
{
    mode_t mode = 0;

    if (flags & O_CREAT) {
	va_list ap;

	va_start(ap, flags);
	mode = (mode_t)va_arg(ap, int);
	va_end(ap);
    }
    return(fullopen("open64", path, flags, mode));
}
#endif

int
mkdir(const char *path, mode_t mode)
{
    static int (*real)(const char *, mode_t);

    if (full(path, "D.")) {
	errno = ENOSPC;
	return(-1);
    }
    if (real == NULL)
	real = (int (*)(const char *, mode_t))dlsym(RTLD_NEXT, "mkdir");
    return(real(path, mode));
}
`

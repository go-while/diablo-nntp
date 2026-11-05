package e2e

import (
	"fmt"
	"github.com/go-while/nntp-testkit/client"
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
	p.ExpectCmd(335, "IHAVE %s", ihave)
	p.MustSend(client.Stuff(art))
	p.ExpectStatus(436, "IHAVE on a full disk")

	p.ExpectCmd(203, "MODE STREAM")
	take := msgID(t)
	takeArt := client.Stuff(newArticle(take, "e2e.test", "full disk takethis", "body"))
	if err := p.Send(fmt.Sprintf("TAKETHIS %s\r\n%s", take, takeArt)); err != nil {
		t.Fatal(err)
	}
	p.ExpectStatus(431, "TAKETHIS on a full disk")

	logged, _ := os.ReadFile(filepath.Join(s.Dir, "log", "general.log"))
	if !strings.Contains(string(logged)+s.Proc.Stderr(), "No space left on device") {
		t.Errorf("no log line about the full disk in log/general.log or stderr:\n%s", logged)
	}

	// Room again: the same articles are taken (they are not in history).
	if err := os.Remove(flag); err != nil {
		t.Fatal(err)
	}
	p.ExpectCmd(335, "IHAVE %s", ihave)
	p.MustSend(client.Stuff(art))
	p.ExpectStatus(235, "IHAVE after room was made")
	if err := p.Send(fmt.Sprintf("TAKETHIS %s\r\n%s", take, takeArt)); err != nil {
		t.Fatal(err)
	}
	p.ExpectStatus(239, "TAKETHIS after room was made")
	p.ExpectCmd(223, "STAT %s", ihave)
	p.ExpectCmd(223, "STAT %s", take)
	p.ExpectCmd(205, "QUIT")
}

// TestFeederFullDiskBacksOff: after 10 articles in a row whose spool file could not be created
// (each answered 431), the session answers 400 and ends, so the peer backs off and reconnects
// later instead of offering article after article that cannot be stored. Before, every
// article got its own 431 (and a second's wait) for as long as the peer kept sending. Once
// there is room again, a new connection takes articles.
func TestFeederFullDiskBacksOff(t *testing.T) {
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
	p.ExpectCmd(203, "MODE STREAM")
	for i := 0; i < 10; i++ {
		id := msgID(t)
		if err := p.Send(fmt.Sprintf("TAKETHIS %s\r\n%s", id, client.Stuff(newArticle(id, "e2e.test", "full disk backoff", "body")))); err != nil {
			t.Fatal(err)
		}
		p.ExpectStatus(431, fmt.Sprintf("TAKETHIS %d on a full disk", i+1))
	}
	line, err := p.ReadLine()
	if err != nil || !strings.HasPrefix(line, "400 ") {
		t.Fatalf("after 10 failed articles: %q (%v), want 400 and the end of the session", line, err)
	}
	if l, err := p.ReadLine(); err == nil {
		t.Errorf("the session went on after the 400: %q", l)
	}
	logged, _ := os.ReadFile(filepath.Join(s.Dir, "log", "general.log"))
	if !strings.Contains(string(logged)+s.Proc.Stderr(), "asking the peer to back off") {
		t.Errorf("no back-off line in log/general.log or stderr:\n%s", logged)
	}

	if err := os.Remove(flag); err != nil {
		t.Fatal(err)
	}
	q := s.Peer(t)
	id := msgID(t)
	q.ExpectCmd(335, "IHAVE %s", id)
	q.MustSend(client.Stuff(newArticle(id, "e2e.test", "after the back-off", "body")))
	q.ExpectStatus(235, "IHAVE after room was made")
	q.ExpectCmd(205, "QUIT")
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

package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	testkit "github.com/go-while/nntp-testkit"
	"github.com/go-while/nntp-testkit/client"
	"github.com/go-while/nntp-testkit/fakeserver"
)

// mkdirRaceSource makes mkdir() of a path that holds MKDIRRACE create the directory and then
// fail with EEXIST, as when another process created it between our stat() and our mkdir().
// Each such call appends the path to the file MKDIRRACE_MARK.
const mkdirRaceSource = `#define _GNU_SOURCE
#define __EXTENSIONS__
#include <dlfcn.h>
#include <errno.h>
#include <fcntl.h>
#include <stdlib.h>
#include <string.h>
#include <sys/stat.h>
#include <sys/types.h>
#include <unistd.h>

int
mkdir(const char *path, mode_t mode)
{
    static int (*real)(const char *, mode_t);
    const char *only = getenv("MKDIRRACE");
    int r;

    if (real == NULL)
	real = (int (*)(const char *, mode_t))dlsym(RTLD_NEXT, "mkdir");
    r = real(path, mode);
    if (r == 0 && only != NULL && strstr(path, only) != NULL) {
	const char *mark = getenv("MKDIRRACE_MARK");
	int fd;

	if (mark != NULL && (fd = open(mark, O_WRONLY|O_CREAT|O_APPEND, 0644)) >= 0) {
	    (void)write(fd, path, strlen(path));
	    (void)write(fd, "\n", 1);
	    close(fd);
	}
	errno = EEXIST;
	return -1;
    }
    return r;
}
`

// TestDeepRevOverviewMkdirRace: dreaderd creates the overview directory of a group when the
// first article of it arrives (MakeGroupDirectory: stat(), then mkdir() of each missing
// level). When another process created a directory between the stat() and the mkdir(), the
// EEXIST of the mkdir() was returned as an error: dreaderd did not open the overview file,
// answered the feed "400 Error writing data" and closed the connection. The directory is
// there, which is what was wanted.
//
// The race is a few instructions wide; the fault injector makes every mkdir() under the
// group directory lose it.
func TestDeepRevOverviewMkdirRace(t *testing.T) {
	so := preloadShim(t, "mkdirrace", mkdirRaceSource)
	mark := filepath.Join(t.TempDir(), "mkdirs")
	st, spool := spoolStore(t)
	s := NewReaderSite(t, ReaderOptions{Spool: spool, NoStart: true})

	// dgrpctl may have made overview directories already: start without them, so that
	// dreaderd makes them for the first article
	home := filepath.Join(s.Dir, "spool", "group")
	ents, err := os.ReadDir(home)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range ents {
		if e.IsDir() {
			if err := os.RemoveAll(filepath.Join(home, e.Name())); err != nil {
				t.Fatal(err)
			}
		}
	}
	startReaderEnv(t, s, lockFaultEnv(so), "MKDIRRACE="+home+"/", "MKDIRRACE_MARK="+mark)

	id := msgID(t)
	art := newArticle(id, "e2e.test", "mkdir race", "body")
	head, body := splitArticle(art)
	st.AddArticle("e2e.test", fakeserver.Article{MessageID: id, Headers: head, Body: body})
	p := s.dialFrom(t, PeerIP)
	p.ExpectCmd(335, "IHAVE %s", id)
	p.MustSend(client.Stuff(art))
	code, line, err := p.ReadStatus()
	if err != nil {
		line = "(" + err.Error() + ")"
	}
	if b, err := os.ReadFile(mark); err != nil || len(b) == 0 {
		t.Fatalf("the scenario did not develop: dreaderd made no directory under %s (%v)", home, err)
	}
	if code != 235 {
		t.Fatalf("IHAVE of the first article of a group whose directory mkdir lost a race: %q, want 235", line)
	}
	p.ExpectCmd(205, "QUIT")

	r := s.Reader(t)
	testkit.WaitFor(t, 10*time.Second, "the article in GROUP e2e.test", func() bool {
		_, l := r.Cmd("GROUP e2e.test")
		f := strings.Fields(l)
		return len(f) > 1 && f[0] == "211" && f[1] == "1"
	})
}

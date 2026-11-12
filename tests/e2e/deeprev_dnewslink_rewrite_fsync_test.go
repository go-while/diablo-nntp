package e2e

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/go-while/nntp-testkit/fakeserver"
)

// fsyncFailSource makes fsync() of a file whose path holds FSYNCFAIL_FILE fail with EIO, and
// then appends that path to the file FSYNCFAIL_MARK: the test sees that the fault was hit.
// With FSYNCFAIL_DIRONLY set only the fsync() of a directory fails.
const fsyncFailSource = `#define _GNU_SOURCE
#define __EXTENSIONS__
#include <dlfcn.h>
#include <errno.h>
#include <fcntl.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/stat.h>
#include <unistd.h>
/*FDPATH*/
int
fsync(int fd)
{
    static int (*real)(int);
    const char *only = getenv("FSYNCFAIL_FILE");
    char path[4096];

    if (real == NULL)
	real = (int (*)(int))dlsym(RTLD_NEXT, "fsync");
    path[0] = 0;
    fdpath(fd, path, sizeof(path));
    if (only != NULL && path[0] != 0 && strstr(path, only) != NULL) {
	const char *mark = getenv("FSYNCFAIL_MARK");
	struct stat st;
	int mfd;

	if (getenv("FSYNCFAIL_DIRONLY") != NULL &&
	    (fstat(fd, &st) != 0 || !S_ISDIR(st.st_mode)))
	    return real(fd);

	if (mark != NULL && (mfd = open(mark, O_WRONLY|O_CREAT|O_APPEND, 0644)) >= 0) {
	    (void)write(mfd, path, strlen(path));
	    (void)write(mfd, "\n", 1);
	    close(mfd);
	}
	errno = EIO;
	return -1;
    }
    return real(fd);
}
`

// TestDeepRevDnewslinkRewriteFsync: dnewslink writes what is left of a queue file into
// <queue>.tmp and renames that over the queue. The data of the new file must be on disk
// before the rename, or a crash in between leaves the queue name with an empty file; so
// dnewslink syncs it, and a failed sync counts as a failed write: the old queue stays.
//
// A crash cannot be made here. The test makes fsync of the rewrite fail instead: without the
// sync, dnewslink never sees the failure and replaces the queue.
func TestDeepRevDnewslinkRewriteFsync(t *testing.T) {
	so := preloadShim(t, "fsyncfail", fsyncFailSource)
	const n = 10
	run := func(t *testing.T, env []string) (string, string) {
		s := NewSite(t, SiteOptions{NoStart: true})
		s.Env = append(s.Env, env...)
		dnlSpool(t, s)
		// the peer does not want the first two articles, then goes away
		checks := 0
		srv := fakeserver.New(t, fakeserver.Options{Greeting: "200 fake", Handler: fakeserver.LineHandler(
			func(_ *fakeserver.Conn, line string) ([]string, bool) {
				l := strings.ToLower(strings.TrimSpace(line))
				switch {
				case strings.HasPrefix(l, "mode stream"):
					return []string{"203 stream ok"}, false
				case strings.HasPrefix(l, "check "):
					checks++
					if checks <= 2 {
						return []string{"438 " + strings.Fields(line)[1]}, false
					}
					return []string{"400 going away"}, true
				}
				return nil, false
			})})
		var b strings.Builder
		for i := 0; i < n; i++ {
			fmt.Fprintf(&b, "D.00000001/B.0001 <q%d@e2e.test> %d,100\n", i, i*100)
		}
		if err := os.WriteFile(filepath.Join(s.Dir, "dqueue", "out.S00000"), []byte(b.String()), 0o644); err != nil {
			t.Fatal(err)
		}
		out, err := s.TryRun("dnewslink", "-N", "1", "-S", "0", "-b", "out.S%05d", "-h", "127.0.0.1",
			"-P", strconv.Itoa(srv.Port()), "-t", "5")
		t.Logf("dnewslink: err=%v\n%s", err, out)
		if _, err := os.Stat(filepath.Join(s.Dir, "dqueue", "out.S00000.tmp")); err == nil {
			t.Errorf("out.S00000.tmp left behind")
		}
		return dnlQueue(t, s, "out.S00000"), generalLog(s)
	}

	t.Run("synced", func(t *testing.T) {
		q, _ := run(t, nil)
		if strings.Contains(q, "<q0@e2e.test>") || strings.Contains(q, "<q1@e2e.test>") {
			t.Fatalf("the scenario did not develop: the queue was not rewritten without the two articles the peer refused:\n%s", q)
		}
		for i := 2; i < n; i++ {
			if !strings.Contains(q, fmt.Sprintf("<q%d@e2e.test>", i)) {
				t.Errorf("<q%d@e2e.test> is not in the rewritten queue", i)
			}
		}
	})
	t.Run("sync fails", func(t *testing.T) {
		mark := filepath.Join(t.TempDir(), "fsynced")
		q, log := run(t, []string{lockFaultEnv(so), "FSYNCFAIL_FILE=/dqueue/out.S00000", "FSYNCFAIL_MARK=" + mark})
		hit, err := os.ReadFile(mark)
		if err != nil || !strings.Contains(string(hit), "/dqueue/out.S00000") {
			t.Errorf("dnewslink did not sync the rewrite of the queue: the fault was never hit (%v)", err)
		}
		for i := 0; i < n; i++ {
			if !strings.Contains(q, fmt.Sprintf("<q%d@e2e.test>", i)) {
				t.Errorf("<q%d@e2e.test> is not in the queue: the rewrite whose sync failed replaced it", i)
			}
		}
		// the log says the old queue is kept, with the error of the sync, not one of a later call
		kept := 0
		for _, l := range strings.Split(log, "\n") {
			if !strings.Contains(l, "keeping the old queue") {
				continue
			}
			kept++
			if !strings.Contains(l, "Input/output error") && !strings.Contains(l, "I/O error") {
				t.Errorf("the log line does not name the I/O error of the sync: %q", l)
			}
		}
		if kept == 0 {
			t.Errorf("dnewslink did not log that it keeps the old queue")
		}
		if t.Failed() {
			t.Logf("queue:\n%s", q)
		}
	})
}

package e2e

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

// sleepGrowSource replaces sleep() with one that returns at once and, on its first call only,
// appends SLEEPGROW_BYTES bytes to the file SLEEPGROW_FILE: what a running diablo writes into
// the spool's newest directory while dexpire sleeps between two passes.
const sleepGrowSource = `#define _GNU_SOURCE
#define __EXTENSIONS__
#include <fcntl.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>
unsigned int
sleep(unsigned int secs)
{
    static int calls;
    const char *file = getenv("SLEEPGROW_FILE");
    const char *bytes = getenv("SLEEPGROW_BYTES");
    char buf[4096];
    long n;
    int fd;

    (void)secs;
    if (calls++ > 0 || file == NULL || bytes == NULL)
	return 0;
    if ((fd = open(file, O_WRONLY | O_CREAT | O_APPEND, 0644)) < 0)
	return 0;
    memset(buf, 'x', sizeof(buf));
    for (n = atol(bytes); n > 0; n -= (long)sizeof(buf)) {
	if (write(fd, buf, n < (long)sizeof(buf) ? (size_t)n : sizeof(buf)) < 0)
	    break;
    }
    close(fd);
    return 0;
}
`

// TestR2DexpireSecondPassHistory (deep review r2 L25, D-267): dexpire runs another pass over
// the spools after one that removed something, and each pass's history update keeps the
// records of the directories that pass listed. The list (NodeAry) was never cleared, so a
// directory listed in pass 1 and removed in pass 2 kept its records unexpired: the feeder
// answered 223 for articles whose files were gone.
//
// Spool 00 with maxsize 1m holds a 1000 KB directory an hour old, the directory of two stored
// articles, and a newer one of 100 KB. Pass 1 removes the old one, which is enough. Between
// the passes (a preloaded sleep() that returns at once) 1 MB is written into the newest
// directory, as a running diablo does, so pass 2 removes the articles' directory. Its records
// must now be marked expired: STAT 430.
func TestR2DexpireSecondPassHistory(t *testing.T) {
	so := preloadShim(t, "sleepgrow", sleepGrowSource)
	s := NewSite(t, SiteOptions{SpoolCtl: "spool 00\n    maxsize 1m\nend\nmetaspool trad\n    spool 00\nend\nexpire * trad\n"})
	ids := s.Inject(t, "e2e.test", 2)
	top := filepath.Join(s.Dir, "spool", "news")
	mid, _ := filepath.Glob(filepath.Join(top, "D.*"))
	if len(mid) != 1 {
		t.Fatalf("spool directories: %q, want one", mid)
	}
	now := time.Now().Unix() / 60
	old := filepath.Join(top, fmt.Sprintf("D.%08x", now-60))
	newest := filepath.Join(top, fmt.Sprintf("D.%08x", now+1))
	for _, d := range []struct {
		dir  string
		size int
	}{{old, 1000 * 1024}, {newest, 100 * 1024}} {
		if err := os.Mkdir(d.dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(d.dir, "B.0001"), make([]byte, d.size), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	grow := filepath.Join(newest, "B.0002")
	base := s.Env
	s.Env = append(slices.Clone(base), lockFaultEnv(so), "SLEEPGROW_FILE="+grow, "SLEEPGROW_BYTES=1048576")
	out, err := s.TryRun("dexpire", "-a", "-h1", "-v")
	s.Env = base
	if err != nil {
		t.Fatalf("dexpire -a -h1: %v\n%s", err, out)
	}
	if fi, err := os.Stat(grow); err != nil || fi.Size() != 1048576 {
		t.Fatalf("test setup: the newest directory did not grow between the passes (%v):\n%s", err, out)
	}
	if _, err := os.Stat(old); err == nil {
		t.Fatalf("test setup: the old directory was not removed:\n%s", out)
	}
	if !strings.Contains(out, "Spool scan pass 2") {
		t.Fatalf("test setup: no second pass:\n%s", out)
	}
	if _, err := os.Stat(mid[0]); err == nil {
		t.Fatalf("test setup: the second pass did not remove %s:\n%s", mid[0], out)
	}
	if _, err := os.Stat(newest); err != nil {
		t.Errorf("the newest directory was removed: %v", err)
	}

	p := s.Peer(t)
	for _, id := range ids {
		if code, line := p.Cmd("STAT %s", id); code != 430 {
			t.Errorf("STAT %s (its directory was removed in pass 2): %q, want 430 (the history record was kept)", id, line)
		}
	}
	p.ExpectCmd(205, "QUIT")
	if t.Failed() {
		t.Logf("dexpire -a -h1 -v:\n%s", out)
	}
}

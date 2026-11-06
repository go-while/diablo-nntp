package e2e

// History findings from /diablo/test/review-history.md, fixed in
// gw-history-batch2: dexpire writing expiry flags into a swapped history (D-189),
// hiscachesize not checked to be a power of two (D-190), HistoryOpen not
// validating the header's entry/header sizes (D-191), and PC_DELCOMM evicting
// another message-id's precommit entry (D-192).

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	testkit "github.com/go-while/nntp-testkit"
	"github.com/go-while/nntp-testkit/client"
)

// chdirHoldSource is an LD_PRELOAD shim: with CHDIRHOLD=dir, chdir() creates
// dir/reached and waits for dir/go before it chdirs. It holds dexpire after it
// opened the history and before it scans it, so the test can swap the history.
const chdirHoldSource = `#define _GNU_SOURCE
#include <dlfcn.h>
#include <fcntl.h>
#include <stdio.h>
#include <stdlib.h>
#include <unistd.h>

int
chdir(const char *path)
{
    int (*real)(const char *) =
	(int (*)(const char *))dlsym(RTLD_NEXT, "chdir");
    const char *dir = getenv("CHDIRHOLD");
    char p[4096];
    int i, fd;

    if (dir != NULL) {
	snprintf(p, sizeof(p), "%s/reached", dir);
	if ((fd = open(p, O_WRONLY|O_CREAT, 0644)) >= 0)
	    close(fd);
	snprintf(p, sizeof(p), "%s/go", dir);
	for (i = 0; i < 6000 && access(p, F_OK) != 0; i++)
	    usleep(10000);
    }
    return real(path);
}
`

// TestDexpireHistorySwappedMidScan (D-189): a running dexpire holds the history
// it opened at the start of the run; a dhisexpire that installs a new history
// meanwhile renames a new inode over the path, so every setExpired write would
// land in the old (dead) file. dexpire now notices that the path no longer names
// its open descriptor and stops. Fails on main: the flags went to the .bak and
// the new history kept the removed articles live.
func TestDexpireHistorySwappedMidScan(t *testing.T) {
	so := preloadShim(t, "chdirhold", chdirHoldSource)
	s := NewSite(t, SiteOptions{})
	s.Inject(t, "e2e.test", 1)
	hist := filepath.Join(s.Dir, "dhistory")
	hold := t.TempDir()

	_, in, out, done := histCmd(t, s, []string{lockFaultEnv(so), "CHDIRHOLD=" + hold}, "dexpire", "-a", "-h1")
	in.Close()
	testkit.WaitFor(t, 30*time.Second, "dexpire at its chdir", func() bool {
		return fileExists(filepath.Join(hold, "reached"))
	})
	fresh := filepath.Join(t.TempDir(), "dhistory")
	s.Run(t, "diload", fresh) // a valid history at a new inode
	if err := os.Rename(fresh, hist); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(hold, "go"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err == nil {
		t.Errorf("dexpire exited 0 after the history was swapped\n%s", out.String())
	} else if !strings.Contains(out.String(), "was replaced") {
		t.Errorf("dexpire did not report the swapped history\n%s", out.String())
	}
}

// TestHiscachesizePowerOfTwo (D-190): a hiscachesize that is not a power of two
// stops dreaderd at startup, as an out-of-range readerdns does, instead of
// leaving the precommit mask with holes. Fails on main: the value was taken
// as-is and only part of the table was ever used.
func TestHiscachesizePowerOfTwo(t *testing.T) {
	_, spool := spoolStore(t)
	for _, c := range []struct{ conf, want string }{
		{"100000", "hiscachesize must be a power of two"},
		{"16385", "hiscachesize must be a power of two"},
		{"0", "hiscachesize must be a power of two"},
		{"bogus", "hiscachesize must be a power of two"},
	} {
		t.Run(c.conf, func(t *testing.T) {
			s := NewReaderSite(t, ReaderOptions{Spool: spool, NoStart: true})
			// the default config has no hiscachesize line: add one.
			conf := s.ReadFile(t, "diablo.config")
			s.WriteFile(t, "diablo.config", conf+"hiscachesize\t"+c.conf+"\n")
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, filepath.Join(s.Bin, "dreaderd"),
				"-d", "-p", PathHost, "-h", PathHost,
				"-P", "[127.0.0.1]:"+strconv.Itoa(testkit.FreePort(t)))
			cmd.Dir = s.Dir
			cmd.Env = append(os.Environ(), s.Env...)
			out, err := cmd.CombinedOutput()
			if ctx.Err() != nil {
				t.Fatalf("dreaderd started with hiscachesize %s; output:\n%s", c.conf, out)
			}
			if err == nil {
				t.Fatalf("dreaderd exited 0 with hiscachesize %s; output:\n%s", c.conf, out)
			}
			if !strings.Contains(string(out), c.want) {
				t.Errorf("no %q error for hiscachesize %s; output:\n%s", c.want, c.conf, out)
			}
		})
	}
}

// TestHistoryOpenBadHeaderSizes (D-191): a history whose header gives an entry
// size or a header size that does not match this build is refused, instead of
// walking the records at the wrong stride (a different ABI or a damaged header).
// Fails on main: only the version and the hash size were checked.
func TestHistoryOpenBadHeaderSizes(t *testing.T) {
	s := NewSite(t, SiteOptions{})
	p := s.Peer(t)
	id := msgID(t)
	p.ExpectCmd(335, "IHAVE %s", id)
	p.MustSend(client.Stuff(newArticle(id, "e2e.test", "history", "body")))
	p.ExpectStatus(235, "IHAVE "+id)
	p.ExpectCmd(205, "QUIT")
	h, err := os.ReadFile(filepath.Join(s.Dir, "dhistory"))
	if err != nil || len(h) < 16 {
		t.Fatalf("dhistory: %v (%d bytes)", err, len(h))
	}
	for _, tc := range []struct {
		name string
		off  int
		val  uint16
	}{
		{"entry size", 10, 4},
		{"header size", 12, 8},
	} {
		t.Run(tc.name, func(t *testing.T) {
			damaged := append([]byte(nil), h...)
			// HistHead: hmagic, hashSize (4 bytes each), version,
			// henSize, headSize (2 bytes each).
			damaged[tc.off], damaged[tc.off+1] = byte(tc.val), byte(tc.val>>8)
			path := filepath.Join(t.TempDir(), "dhistory")
			if err := os.WriteFile(path, damaged, 0o644); err != nil {
				t.Fatal(err)
			}
			out, err := s.TryRun("dilookup", "-h", path, "03456789.0abcdef0")
			var ee *exec.ExitError
			if !errors.As(err, &ee) || ee.ExitCode() != 1 {
				t.Errorf("%s %d: dilookup %v, want exit status 1\n%s", tc.name, tc.val, err, out)
			}
			if !strings.Contains(out, "does not match this build") {
				t.Errorf("%s %d: dilookup did not report the mismatch:\n%s", tc.name, tc.val, out)
			}
		})
	}
}

// pcDriver is the C driver for TestPrecommitDelcommLeavesOtherSlot. With
// PCHSize 1 (PCHMask 0) every message-id shares slot 0, so a DELCOMM of one id
// lands on the slot of another.
const pcDriver = `#include "defs.h"

extern int PCPid;

int
main(void)
{
    int r;

    SetPCommitExpire(30, 45 * 60, 1);	/* PCHSize 1: every id shares slot 0 */
    InitPreCommit();

    /*
     * B is precommitted into slot 0 by one feed child (a PRECOMM, not a
     * POSTCOMM: without DO_PCOMMIT_POSTCACHE, as on NetBSD, a POSTCOMM
     * stores nothing).
     */
    PCPid = 1000001;
    (void)PreCommit("<b@x>", PC_PRECOMM);
    /* Another child's DELCOMM of A, which is not in the slot: B must stay. */
    PCPid = 1000002;
    (void)PreCommit("<a@x>", PC_DELCOMM);
    /* B is still precommitted by the first child: -2 for the second. */
    r = PreCommit("<b@x>", PC_PRECOMM);
    printf("delcomm preserves another message-id: %s\n", r == -2 ? "yes" : "no");

    /*
     * Our own entry: a precommit, then a DELCOMM from the same process (a
     * reject) must clear it, so another process's CHECK is not refused.
     * PCPid is the precommitting process; a different value is another
     * feed child.
     */
    PCPid = 1000003;
    (void)PreCommit("<c@x>", PC_PRECOMM);
    (void)PreCommit("<c@x>", PC_DELCOMM);
    PCPid = 1000004;
    r = PreCommit("<c@x>", PC_PRECOMM);
    printf("delcomm clears our own entry: %s\n", r == 0 ? "yes" : "no");
    return 0;
}
`

// pcDrvBuild compiles pcDriver against the tree's libdiablo.a (it must have been
// built) and returns the executable.
func pcDrvBuild(t *testing.T) string {
	t.Helper()
	return libdiabloDriver(t, pcDriver, "pcdrv", "precommit driver")
}

// TestPrecommitDelcommLeavesOtherSlot (D-192): a PC_DELCOMM for a message-id
// whose hash slot holds a different message-id left that slot alone instead of
// clearing it, so the other article's precommit/postcommit stays live. Fails on
// main: the cleared entry replaced the other id, and the next lookup missed it.
func TestPrecommitDelcommLeavesOtherSlot(t *testing.T) {
	exe := pcDrvBuild(t)
	out, err := exec.Command(exe).CombinedOutput()
	if err != nil {
		t.Fatalf("precommit driver: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "delcomm preserves another message-id: yes") {
		t.Errorf("the DELCOMM evicted the other message-id's entry:\n%s", out)
	}
	if !strings.Contains(string(out), "delcomm clears our own entry: yes") {
		t.Errorf("the DELCOMM left our own precommit entry, so another process's CHECK is refused:\n%s", out)
	}
}

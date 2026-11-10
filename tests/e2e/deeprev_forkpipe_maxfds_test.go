package e2e

import (
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	testkit "github.com/go-while/nntp-testkit"
)

// TestDeepRevForkPipeMaxFds: the main dreaderd process keeps the socketpair to each reader
// and DNS worker in its descriptor table, which ends before MAXFDS (AddThread() exits the
// process for a descriptor at or above it). ForkPipe() refused a pair only above MAXFDS, so a
// descriptor equal to MAXFDS got through and the main process exited.
//
// MAXFDS is the size of the select() set (1024 with glibc, up to 3960 elsewhere). A preloaded
// socketpair() moves the main side of the first pair of the main process to a chosen number. A first run moves it to 6000, which both
// versions refuse with a log line that tells MAXFDS. The second run moves it to MAXFDS: the
// main process must refuse the pair, start the process with a new one and serve clients.
func TestDeepRevForkPipeMaxFds(t *testing.T) {
	so := preloadShim(t, "highfd", withMark(highFdSource))
	refusedRe := regexp.MustCompile(`pipe desc >=? MAXFDS: (\d+)/(\d+)/(\d+)`)
	run := func(t *testing.T, at int) (*Site, string) {
		mark := filepath.Join(t.TempDir(), "mark")
		_, spool := spoolStore(t)
		s := NewReaderSite(t, ReaderOptions{Spool: spool, NoStart: true})
		s.Env = append(s.Env, lockFaultEnv(so), "SHIM_MARK="+mark, "FDHIGH_PAIR=1", "FDHIGH_AT="+strconv.Itoa(at))
		t.Cleanup(func() {
			if t.Failed() {
				t.Logf("general.log:\n%s\nmarks:\n%s", generalLog(s), strings.Join(shimMarks(mark), "\n"))
			}
		})
		startReaderArgs(t, s)
		testkit.WaitFor(t, testkit.Deadline(), "the injector to move the descriptor", func() bool {
			return len(shimMarks(mark)) > 0
		})
		if m := shimMarked(mark, "cannot move"); len(m) > 0 {
			t.Skipf("the descriptor could not be moved to %d: %s", at, m[0])
		}
		if len(shimMarked(mark, "to "+strconv.Itoa(at))) == 0 {
			t.Fatalf("the injector did not move a descriptor to %d", at)
		}
		return s, waitGreeting(s.Addr, 15*time.Second)
	}

	var maxfds int
	ok := t.Run("above MAXFDS", func(t *testing.T) {
		s, g := run(t, 6000)
		m := refusedRe.FindStringSubmatch(generalLog(s))
		if m == nil || m[1] != "6000" {
			t.Fatalf("descriptor 6000 was not refused (no \"pipe desc > MAXFDS: 6000/...\" in the log)")
		}
		maxfds, _ = strconv.Atoi(m[3])
		if !strings.HasPrefix(g, "200") {
			t.Errorf("greeting %q after the refused pair, want 200", g)
		}
	})
	if maxfds == 0 {
		if ok { // the probe was skipped: the descriptor could not be moved
			t.Skip("MAXFDS is not known: the probe was skipped")
		}
		t.Fatal("MAXFDS is not known")
	}
	t.Run("equal to MAXFDS", func(t *testing.T) {
		s, g := run(t, maxfds)
		if !strings.HasPrefix(g, "200") {
			t.Errorf("descriptor %d (MAXFDS) for a socketpair: greeting %q, want 200 (the main process is gone)", maxfds, g)
		}
		if m := refusedRe.FindStringSubmatch(generalLog(s)); m == nil || m[1] != strconv.Itoa(maxfds) {
			t.Errorf("descriptor %d (MAXFDS) for a socketpair was not refused", maxfds)
		}
		if strings.Contains(generalLog(s), "AddThread: descriptor") {
			t.Errorf("AddThread() got the descriptor: %q in the log", "AddThread: descriptor")
		}
	})
}

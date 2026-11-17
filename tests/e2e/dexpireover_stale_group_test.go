package e2e

// D-252 (deep review round 2, L10): "dexpireover -O" deletes a group whose last article
// (LMTS) is older than the given number of days, judged by the LMTS read at the start of the
// run (util/dexpireover.c deleteStaleGroup).

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

// TestDexpireoverStaleGroupNewArticle: e2e.test and e2e.old both have a last-article time ten
// days back. dexpireover -a -s -O1 -w e2e.* reads them, then waits in its pass over the directories for
// the lock on the over file of e2e.test, which the test holds. Meanwhile e2e.test gets an
// article (its LMTS is set to now, as dreaderd does when it numbers one). At the end of the
// run e2e.test must still be in dactive.kp and e2e.old must be gone. Fails before the fix:
// both were deleted, by the LMTS read at the start.
func TestDexpireoverStaleGroupNewArticle(t *testing.T) {
	s, home := overSite(t, nil)
	s.Run(t, "dgrpctl", "adjust", "e2e.test", "NE", "=1000")
	s.WriteFile(t, "dexpire.ctl", "e2e.*:x30\n")
	db := filepath.Join(s.Dir, "dactive.kp")
	dkp := func(a ...string) {
		t.Helper()
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		c := exec.CommandContext(ctx, filepath.Join(s.Bin, "dkp"), a...)
		c.Dir = s.Dir
		c.Env = append(os.Environ(), s.Env...)
		if o, err := c.CombinedOutput(); err != nil {
			t.Fatalf("dkp %s: %v\n%s", strings.Join(a, " "), err, o)
		}
	}
	lmts := func(at time.Time) string { return fmt.Sprintf("LMTS=%08x", at.Unix()) }
	old := time.Now().Add(-10 * 24 * time.Hour)
	dkp("-w", db, "e2e.test", lmts(old))
	for _, f := range []string{"NB=0000000001", "NE=0000000010", "S=y", lmts(old)} {
		dkp("-w", db, "e2e.old", f)
	}

	f := strings.Fields(s.Run(t, "dgrpctl", "listgrouphashfile", "e2e.test"))
	if len(f) != 2 || !strings.HasPrefix(filepath.Base(f[0]), "over.0.") {
		t.Fatalf("dgrpctl listgrouphashfile e2e.test: %q", f)
	}
	over := filepath.Join(home, f[0])
	if err := os.MkdirAll(filepath.Dir(over), 0o755); err != nil {
		t.Fatal(err)
	}
	// 500 slots for NB..NE 1..1000: -s resizes, and waits for the lock at offset 4
	writeOver(t, over, append(overHeadMax("e2e.test", 500), make([]byte, 500*32)...))
	fo, err := os.OpenFile(over, os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer fo.Close()
	lk := syscall.Flock_t{Type: syscall.F_WRLCK, Whence: 0, Start: 4, Len: 4}
	if err := syscall.FcntlFlock(fo.Fd(), syscall.F_SETLK, &lk); err != nil {
		t.Fatalf("lock %s: %v", over, err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, filepath.Join(s.Bin, "dexpireover"), "-a", "-s", "-O1", "-w", "e2e.*", "-v", "-l60")
	cmd.Dir = s.Dir
	cmd.Env = append(os.Environ(), s.Env...)
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	if err := waitLockWaiter(cmd.Process.Pid, done); err != nil {
		t.Fatalf("dexpireover did not wait for the lock: %v\n%s", err, out.String())
	}

	// The group table (with both LMTS values) has been read: e2e.test gets an article now.
	dkp("-w", db, "e2e.test", lmts(time.Now()))

	lk.Type = syscall.F_UNLCK
	if err := syscall.FcntlFlock(fo.Fd(), syscall.F_SETLK, &lk); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatalf("dexpireover: %v\n%s", err, out.String())
	}
	if live, _, _ := kpRecords(t, db, "e2e.test"); len(live) != 1 {
		t.Errorf("dexpireover -O1 deleted e2e.test, which got an article during the run: records %q", live)
	}
	if live, _, _ := kpRecords(t, db, "e2e.old"); len(live) != 0 {
		t.Errorf("dexpireover -O1 kept e2e.old, whose last article is ten days old: records %q", live)
	}
	if t.Failed() {
		t.Logf("dexpireover -a -s -O1 -w e2e.* -v -l60:\n%s", out.String())
	}
}

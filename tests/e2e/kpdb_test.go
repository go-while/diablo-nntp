package e2e

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	testkit "github.com/go-while/nntp-testkit"
)

// kpRecords returns the live ('+') and deleted ('-') records of key in a KP database, and
// the header line.
func kpRecords(tb testing.TB, path, key string) (live, dead []string, header string) {
	tb.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		tb.Fatal(err)
	}
	lines := strings.Split(strings.TrimSuffix(string(b), "\n"), "\n")
	header = lines[0]
	for _, l := range lines[1:] {
		_, rest, ok := strings.Cut(l, ":")
		if !ok {
			continue
		}
		k, _, _ := strings.Cut(rest, " ")
		if k != key {
			continue
		}
		switch l[0] {
		case '+':
			live = append(live, l)
		case '-':
			dead = append(dead, l)
		}
	}
	return live, dead, header
}

// TestKPDBConcurrentUpdate: two writers update the same record with values of a new length
// (so each appends a merged copy and deletes the old one) while the record's lock makes the
// second one wait inside its first lookup. When it gets the lock, the record it found has
// been replaced; it must merge from, and delete, the replacement. Before #33 it wrote the
// "-" at the offset from its first lookup, which was already deleted, and left two live
// records for the key (one without the first writer's change), as a group in dactive.kp
// can end up listed twice. Uses dkp, and holds the record lock (a 4-byte fcntl lock at the
// record's offset, as hflock() takes it) from the test.
func TestKPDBConcurrentUpdate(t *testing.T) {
	b := bin(t)
	dkp := filepath.Join(b, "dkp")
	if _, err := os.Stat(dkp); err != nil {
		t.Skipf("no dkp in %s", b)
	}
	db := filepath.Join(t.TempDir(), "test.kp")
	run := func(args ...string) {
		t.Helper()
		if out, err := exec.Command(dkp, args...).CombinedOutput(); err != nil {
			t.Fatalf("dkp %s: %v\n%s", strings.Join(args, " "), err, out)
		}
	}
	run("-w", db, "other", "x=1")
	run("-w", db, "k", "a=1")

	data, err := os.ReadFile(db)
	if err != nil {
		t.Fatal(err)
	}
	off := bytes.Index(data, []byte("\n+00000000.0000:k "))
	if off < 0 {
		t.Fatalf("no record for k in %q", data)
	}
	off++ // the record starts after the newline

	f, err := os.OpenFile(db, os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	lk := syscall.Flock_t{Type: syscall.F_WRLCK, Whence: 0, Start: int64(off), Len: 4}
	if err := syscall.FcntlFlock(f.Fd(), syscall.F_SETLK, &lk); err != nil {
		t.Fatalf("lock record k: %v", err)
	}

	// Both writers find k and wait for its record lock, which the test holds. A writer that
	// does not wait for it, or that has finished before the unlock, does not run the
	// path this test is for.
	st, err := f.Stat()
	if err != nil {
		t.Fatal(err)
	}
	ino := uint64(st.Sys().(*syscall.Stat_t).Ino)
	ctx, cancel := context.WithTimeout(context.Background(), 2*testkit.Deadline())
	defer cancel()
	var writers []*exec.Cmd
	var outs []*bytes.Buffer
	var dones []chan error
	for _, tv := range []string{"a=22", "b=333"} {
		cmd := exec.CommandContext(ctx, dkp, "-w", db, "k", tv)
		var out bytes.Buffer
		cmd.Stdout, cmd.Stderr = &out, &out
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		done := make(chan error, 1)
		go func() { done <- cmd.Wait() }()
		writers = append(writers, cmd)
		outs = append(outs, &out)
		dones = append(dones, done)
	}
	// output stops the writers that still run and returns writer i's output once every
	// writer has exited, so a buffer is never read while its writer writes to it.
	exited := make([]bool, len(writers))
	output := func(i int) string {
		cancel()
		for j := range writers {
			if !exited[j] {
				<-dones[j]
				exited[j] = true
			}
		}
		return outs[i].String()
	}
	for i, w := range writers {
		if err := waitRecordWaiter(w.Process.Pid, ino, int64(off), dones[i]); err != nil {
			t.Fatalf("writer %d did not wait while the record was locked: %v\n%s", i, err, output(i))
		}
	}
	for i := range writers {
		select {
		case err := <-dones[i]:
			exited[i] = true
			t.Fatalf("writer %d finished while the record was locked (%v):\n%s", i, err, output(i))
		default:
		}
	}

	lk.Type = syscall.F_UNLCK
	if err := syscall.FcntlFlock(f.Fd(), syscall.F_SETLK, &lk); err != nil {
		t.Fatal(err)
	}
	for i := range writers {
		err := <-dones[i]
		exited[i] = true
		if err != nil {
			t.Fatalf("writer %d: %v\n%s", i, err, output(i))
		}
	}

	live, dead, header := kpRecords(t, db, "k")
	if !strings.HasPrefix(header, "$V") {
		t.Errorf("header damaged: %q", header)
	}
	if len(live) != 1 {
		t.Fatalf("%d live records for k, want 1:\n%s\ndeleted:\n%s", len(live), strings.Join(live, "\n"), strings.Join(dead, "\n"))
	}
	if !strings.Contains(live[0], " a=22") || !strings.Contains(live[0], " b=333") {
		t.Errorf("live record %q lost one writer's change (want a=22 and b=333)", live[0])
	}
	if otherLive, _, _ := kpRecords(t, db, "other"); len(otherLive) != 1 {
		t.Errorf("record other: %d live records, want 1", len(otherLive))
	}
}

// waitRecordWaiter returns once process pid waits for the lock on the 4 bytes at off of the
// file with inode ino. On Linux /proc/locks shows the wait ("1: -> POSIX ADVISORY WRITE
// pid maj:min:inode start end"), so a wait for any other lock does not count. Elsewhere, or
// with E2E_LOCKWAIT set to another mode, it falls back to waitLockWaiter, which accepts a
// wait for any POSIX lock of that process (and on a system with none of its tools assumes
// the wait after 2 s). It fails when the process exits first.
func waitRecordWaiter(pid int, ino uint64, off int64, done chan error) error {
	if m := os.Getenv("E2E_LOCKWAIT"); (m != "" && m != "proc") || !fileExists("/proc/locks") {
		return waitLockWaiter(pid, done)
	}
	wpid, wino := strconv.Itoa(pid), ":"+strconv.FormatUint(ino, 10)
	wstart, wend := strconv.FormatInt(off, 10), strconv.FormatInt(off+3, 10)
	other := ""
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		b, _ := os.ReadFile("/proc/locks")
		for _, l := range strings.Split(string(b), "\n") {
			fs := strings.Fields(l)
			if len(fs) > 8 && fs[1] == "->" && fs[2] == "POSIX" && fs[5] == wpid {
				if strings.HasSuffix(fs[6], wino) && fs[7] == wstart && fs[8] == wend {
					return nil
				}
				other = strings.TrimSpace(l)
			}
		}
		select {
		case err := <-done:
			done <- err
			return fmt.Errorf("exited first (%v)", err)
		case <-time.After(20 * time.Millisecond):
		}
	}
	if other != "" {
		return fmt.Errorf("not seen waiting for the record lock at %d after 30s; last wait seen: %q", off, other)
	}
	return fmt.Errorf("not seen waiting for the record lock at %d in /proc/locks after 30s", off)
}

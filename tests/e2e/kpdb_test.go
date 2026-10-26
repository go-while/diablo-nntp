package e2e

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
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

	// Both writers find k and wait for its lock.
	ctx, cancel := context.WithTimeout(context.Background(), 2*testkit.Deadline())
	defer cancel()
	var writers []*exec.Cmd
	var outs []*bytes.Buffer
	for _, tv := range []string{"a=22", "b=333"} {
		cmd := exec.CommandContext(ctx, dkp, "-w", db, "k", tv)
		var out bytes.Buffer
		cmd.Stdout, cmd.Stderr = &out, &out
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		writers = append(writers, cmd)
		outs = append(outs, &out)
	}
	time.Sleep(time.Second)
	for i, w := range writers {
		if w.ProcessState != nil {
			t.Fatalf("writer %d finished while the record was locked:\n%s", i, outs[i])
		}
	}

	lk.Type = syscall.F_UNLCK
	if err := syscall.FcntlFlock(f.Fd(), syscall.F_SETLK, &lk); err != nil {
		t.Fatal(err)
	}
	for i, w := range writers {
		if err := w.Wait(); err != nil {
			t.Fatalf("writer %d: %v\n%s", i, err, outs[i])
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

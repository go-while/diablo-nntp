package e2e

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// TestR2DrequeueRotatedQueue (deep review r2 M11, D-238): drequeue opened dqueue/<label> once
// with O_APPEND (util/drequeue.c writeFeedOut) and never looked at it again. dspoolout renames
// the file to <label>.S<seq> (util/dspoolout.c) and dnewslink removes it when it has sent it,
// so everything drequeue wrote after the rotation went into a file that was gone, and drequeue
// still exited 0. drequeue now opens the queue file again by name before a write when the name
// points at another file, or at none.
//
// Here drequeue gets SOUT lines from a pipe in three batches. Between them the queue file is
// rotated as dspoolout does, read and removed as dnewslink does; at the second rotation
// dspoolout's empty new queue file (realtime feeds) is created as well. Every Message-ID must
// reach a queue file exactly once: one of the rotated files before its removal, or dqueue/peer
// at the end. Fails on main: the lines of the second and third batch go to removed files.
func TestR2DrequeueRotatedQueue(t *testing.T) {
	r := startR2M11(t)
	r.send(1500)
	r.rotate(1, true, false)
	r.send(1500)
	r.rotate(2, false, true) // on main there is no dqueue/peer left to rename
	r.send(1500)
	lost, err := r.finish()
	if err != nil {
		t.Fatalf("drequeue: %v\n%.400s", err, r.out.String())
	}
	r.check(lost)
}

// TestR2DrequeueReopenFails (M11, round 0): after a rotation the queue file cannot be made again
// (dqueue/ is not writable for a while). drequeue must not write to the file it held, which the
// name has left and dnewslink removes: it keeps the lines in its buffer and tries again on the
// next flush, says once that the file cannot be opened, and at the end reports the lines that did
// not fit meanwhile and exits 1. Every Message-ID is either in a queue file or counted in that
// report. Fails on main: the lines go to the removed file and drequeue exits 0.
func TestR2DrequeueReopenFails(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root can create files in a directory without write permission")
	}
	r := startR2M11(t)
	dq := filepath.Dir(r.q)
	r.send(1500)
	r.rotate(1, true, false)
	if err := os.Chmod(dq, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dq, 0o755) })
	r.send(3000) // more than one queue buffer (up to 64 KB)
	r.waitOpenFailure()
	r2m11Idle(t, r.q)
	if err := os.Chmod(dq, 0o755); err != nil {
		t.Fatal(err)
	}
	r.send(1500)
	lost, err := r.finish()
	var ee *exec.ExitError
	if !errors.As(err, &ee) || ee.ExitCode() != 1 {
		t.Errorf("drequeue after a failed reopen: %v, want exit status 1\n%.600s", err, r.out.String())
	}
	if lost == 0 {
		t.Errorf("drequeue reported no lines as not queued:\n%.600s", r.out.String())
	}
	if n := len(r2m11OpenFailure.FindAllString(r.out.String(), -1)); n != 1 {
		t.Errorf("the open failure was reported %d times, want once:\n%.600s", n, r.out.String())
	}
	r.check(lost)
}

// TestR2DrequeueOpenFails (M11, round 0): the queue file cannot be opened at all. drequeue used to
// drop every line for that label without a word and exit 0; now it says so, reports the lines
// that were not queued and exits 1.
func TestR2DrequeueOpenFails(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root can create files in a directory without write permission")
	}
	r := startR2M11(t)
	dq := filepath.Dir(r.q)
	if err := os.Chmod(dq, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dq, 0o755) })
	r.send(10)
	lost, err := r.finish()
	var ee *exec.ExitError
	if !errors.As(err, &ee) || ee.ExitCode() != 1 {
		t.Errorf("drequeue without a queue file: %v, want exit status 1\n%.600s", err, r.out.String())
	}
	if lost != 10 || !r2m11OpenFailure.MatchString(r.out.String()) {
		t.Errorf("drequeue reported %d lines as not queued, want 10, and the open failure:\n%.600s", lost, r.out.String())
	}
}

// drequeue's own lines on stderr (log lines start with a time stamp instead).
var (
	r2m11OpenFailure = regexp.MustCompile(`(?m)^drequeue: peer: unable to open the queue file`)
	r2m11Lost        = regexp.MustCompile(`(?m)^drequeue: peer: (\d+) lines were not queued`)
)

// r2m11Run is a drequeue peer run fed SOUT lines on a pipe.
type r2m11Run struct {
	t    *testing.T
	cmd  *exec.Cmd
	in   io.WriteCloser
	out  *r2m11Buf
	q    string
	next int
	seen map[string]int
}

// r2m11Buf collects drequeue's output; the test reads it while drequeue runs.
type r2m11Buf struct {
	mu sync.Mutex
	b  strings.Builder
}

func (b *r2m11Buf) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.b.Write(p)
}

func (b *r2m11Buf) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.b.String()
}

func startR2M11(t *testing.T) *r2m11Run {
	t.Helper()
	s := NewSite(t, SiteOptions{NoStart: true})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)
	r := &r2m11Run{t: t, out: &r2m11Buf{}, q: filepath.Join(s.Dir, "dqueue", "peer"), seen: map[string]int{}}
	r.cmd = exec.CommandContext(ctx, filepath.Join(s.Bin, "drequeue"), "peer")
	r.cmd.Dir = s.Dir
	r.cmd.Env = append(os.Environ(), s.Env...)
	in, err := r.cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	r.in = in
	r.cmd.Stdout, r.cmd.Stderr = r.out, r.out
	if err := r.cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = r.cmd.Process.Kill(); _ = r.cmd.Wait() })
	return r
}

// send writes n more SOUT lines (about 50 bytes of queue file each).
func (r *r2m11Run) send(n int) {
	r.t.Helper()
	for end := r.next + n; r.next < end; r.next++ {
		line := strings.Join([]string{"SOUT", "D.00000000/B.0000", "0,10", r2m11ID(r.next),
			"e2e.test", "world", "e2e.other!not-for-mail", "0", "000000", "0"}, "\t") + "\n"
		if _, err := io.WriteString(r.in, line); err != nil {
			r.t.Fatalf("write to drequeue: %v\n%.400s", err, r.out.String())
		}
	}
}

func (r *r2m11Run) count(b []byte) {
	for _, line := range strings.Split(strings.TrimRight(string(b), "\n"), "\n") {
		if f := strings.Fields(line); len(f) >= 2 {
			r.seen[f[1]]++
		}
	}
}

// rotate renames dqueue/peer as dspoolout does (and with recreate makes dspoolout's new empty
// file), waits for a write that was under way, then reads (dnewslink sends it) and removes the
// rotated file. With strict a missing dqueue/peer fails the test: the rotation was not exercised.
func (r *r2m11Run) rotate(seq int, strict, recreate bool) {
	r.t.Helper()
	r2m11Idle(r.t, r.q)
	rot := fmt.Sprintf("%s.S%05d", r.q, seq)
	if err := os.Rename(r.q, rot); err != nil {
		if !os.IsNotExist(err) || strict {
			r.t.Fatalf("rotation %d: %v", seq, err)
		}
		r.t.Logf("rotation %d: no dqueue/peer to rename", seq)
		return
	}
	if recreate {
		if err := os.WriteFile(r.q, nil, 0o644); err != nil {
			r.t.Fatal(err)
		}
	}
	time.Sleep(500 * time.Millisecond)
	b, err := os.ReadFile(rot)
	if err != nil {
		r.t.Fatal(err)
	}
	r.count(b)
	if err := os.Remove(rot); err != nil {
		r.t.Fatal(err)
	}
}

// waitOpenFailure waits until drequeue says that it cannot open the queue file.
func (r *r2m11Run) waitOpenFailure() {
	r.t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if r2m11OpenFailure.MatchString(r.out.String()) {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	r.t.Fatalf("drequeue did not report the open failure within 15 s:\n%.600s", r.out.String())
}

// finish closes drequeue's input, waits for it, counts dqueue/peer and returns the number of
// lines drequeue reported as not queued.
func (r *r2m11Run) finish() (int, error) {
	r.in.Close()
	err := r.cmd.Wait()
	if b, rerr := os.ReadFile(r.q); rerr == nil {
		r.count(b)
	}
	lost := 0
	if m := r2m11Lost.FindStringSubmatch(r.out.String()); m != nil {
		lost, _ = strconv.Atoi(m[1])
	}
	return lost, err
}

// check: every Message-ID sent reached a queue file that was read exactly once, or is one of
// the lines drequeue reported as not queued.
func (r *r2m11Run) check(lost int) {
	r.t.Helper()
	missing, twice := 0, 0
	for i := 0; i < r.next; i++ {
		switch n := r.seen[r2m11ID(i)]; {
		case n == 0:
			missing++
		case n > 1:
			twice++
		}
	}
	if missing != lost || twice != 0 || len(r.seen) != r.next-missing {
		r.t.Errorf("of %d Message-IDs %d reached no queue file that was read (drequeue reported %d as not queued), %d reached one twice (%d distinct ids seen)",
			r.next, missing, lost, twice, len(r.seen))
	}
}

func r2m11ID(i int) string { return fmt.Sprintf("<r2m11-%05d@e2e.test>", i) }

// r2m11Idle waits until drequeue has taken a batch: the queue file's size (or its absence)
// stays the same for 500 ms. drequeue keeps the last part of a batch in its buffer until more
// lines come, so nothing is written meanwhile.
func r2m11Idle(t *testing.T, q string) {
	t.Helper()
	last, stable := int64(-2), 0
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		size := int64(-1)
		if st, err := os.Stat(q); err == nil {
			size = st.Size()
		}
		if size == last {
			if stable++; stable >= 5 {
				return
			}
		} else {
			last, stable = size, 0
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("%s kept changing for 15 s", q)
}

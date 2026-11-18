package e2e

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestDrequeueRotatedQueue (D-238): drequeue opened dqueue/<label> once
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
// at the end. Fails on main: the lines written after the first rotation go to the removed file.
func TestDrequeueRotatedQueue(t *testing.T) {
	s := NewSite(t, SiteOptions{NoStart: true})
	sout := func(i int) string {
		return strings.Join([]string{"SOUT", "D.00000000/B.0000", "0,10", drequeueID(i),
			"e2e.test", "world", "e2e.other!not-for-mail", "0", "000000", "0"}, "\t") + "\n"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, filepath.Join(s.Bin, "drequeue"), "peer")
	cmd.Dir = s.Dir
	cmd.Env = append(os.Environ(), s.Env...)
	in, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	var out strings.Builder
	cmd.Stdout, cmd.Stderr = &out, &out
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })

	// 1500 lines of about 50 bytes are more than one queue buffer (one page, up to 64 KB),
	// so drequeue writes during every batch, not only at the end.
	const batch = 1500
	next := 0
	send := func() {
		for end := next + batch; next < end; next++ {
			if _, err := io.WriteString(in, sout(next)); err != nil {
				t.Fatalf("write to drequeue: %v\n%.400s", err, out.String())
			}
		}
	}
	q := filepath.Join(s.Dir, "dqueue", "peer")
	seen := map[string]int{}
	count := func(b []byte) {
		for _, line := range strings.Split(strings.TrimRight(string(b), "\n"), "\n") {
			if f := strings.Fields(line); len(f) >= 2 {
				seen[f[1]]++
			}
		}
	}
	// rotate renames dqueue/peer as dspoolout does, waits for a write that was under way,
	// then reads (dnewslink sends it) and removes the rotated file.
	rotate := func(seq int, recreate bool) {
		drequeueIdle(t, q)
		rot := fmt.Sprintf("%s.S%05d", q, seq)
		if err := os.Rename(q, rot); err != nil {
			if !os.IsNotExist(err) {
				t.Fatal(err)
			}
			t.Logf("rotation %d: no dqueue/peer to rename", seq)
			return
		}
		if recreate {
			if err := os.WriteFile(q, nil, 0o644); err != nil {
				t.Fatal(err)
			}
		}
		time.Sleep(500 * time.Millisecond)
		b, err := os.ReadFile(rot)
		if err != nil {
			t.Fatal(err)
		}
		count(b)
		if err := os.Remove(rot); err != nil {
			t.Fatal(err)
		}
	}

	send()
	rotate(1, false)
	send()
	rotate(2, true)
	send()
	in.Close()
	if err := cmd.Wait(); err != nil {
		t.Fatalf("drequeue: %v\n%.400s", err, out.String())
	}
	if b, err := os.ReadFile(q); err == nil {
		count(b)
	}
	lost, twice := 0, 0
	for i := 0; i < next; i++ {
		switch n := seen[drequeueID(i)]; {
		case n == 0:
			lost++
		case n > 1:
			twice++
		}
	}
	if lost != 0 || twice != 0 || len(seen) != next {
		t.Errorf("drequeue exited 0, but of %d Message-IDs %d reached no queue file that was read, %d reached one twice (%d distinct ids seen)",
			next, lost, twice, len(seen))
	}
}

func drequeueID(i int) string { return fmt.Sprintf("<dqrot-%05d@e2e.test>", i) }

// drequeueIdle waits until drequeue has taken a batch: the queue file's size (or its absence)
// stays the same for 500 ms. drequeue keeps the last part of a batch in its buffer until more
// lines come, so nothing is written meanwhile.
func drequeueIdle(t *testing.T, q string) {
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

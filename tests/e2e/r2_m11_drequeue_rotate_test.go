package e2e

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	testkit "github.com/go-while/nntp-testkit"
)

// TestR2DrequeueRotatedQueue (deep review r2 M11): drequeue opens dqueue/<label> once with
// O_APPEND (util/drequeue.c writeFeedOut) and never looks at it again. dspoolout renames the
// file to <label>.S<seq> (util/dspoolout.c) and dnewslink removes it when it has sent it, so
// everything drequeue writes after the rotation goes into a file that is gone, and drequeue
// still exits 0. Here drequeue gets SOUT lines from a pipe; after its first write the queue
// file is rotated as dspoolout does, read and removed as dnewslink does, and more lines
// follow. Every line must reach a queue file: the rotated one before its removal, or
// dqueue/peer afterwards.
func TestR2DrequeueRotatedQueue(t *testing.T) {
	s := NewSite(t, SiteOptions{NoStart: true})
	sout := func(i int) string {
		return strings.Join([]string{"SOUT", "D.00000000/B.0000", "0,10", fmt.Sprintf("<r2m11-%04d@e2e.test>", i),
			"e2e.test", "world", "e2e.other!not-for-mail", "0", "000000", "0"}, "\t") + "\n"
	}
	cmd := exec.Command(filepath.Join(s.Bin, "drequeue"), "peer")
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
	t.Cleanup(func() { _ = cmd.Process.Kill() })

	const first, second = 300, 300
	for i := 0; i < first; i++ {
		if _, err := io.WriteString(in, sout(i)); err != nil {
			t.Fatal(err)
		}
	}
	q := filepath.Join(s.Dir, "dqueue", "peer")
	testkit.WaitFor(t, 10*time.Second, "drequeue's first write to dqueue/peer", func() bool {
		st, err := os.Stat(q)
		return err == nil && st.Size() > 0
	})
	rot := q + ".S00001"
	if err := os.Rename(q, rot); err != nil { // dspoolout
		t.Fatal(err)
	}
	b, err := os.ReadFile(rot) // dnewslink sends it ...
	if err != nil {
		t.Fatal(err)
	}
	sent := strings.Count(string(b), "\n")
	if err := os.Remove(rot); err != nil { // ... and removes it
		t.Fatal(err)
	}

	for i := first; i < first+second; i++ {
		if _, err := io.WriteString(in, sout(i)); err != nil {
			t.Fatal(err)
		}
	}
	in.Close()
	if err := cmd.Wait(); err != nil {
		t.Fatalf("drequeue: %v\n%.400s", err, out.String())
	}
	queued := 0
	if b, err := os.ReadFile(q); err == nil {
		queued = strings.Count(string(b), "\n")
	}
	if sent+queued != first+second {
		t.Errorf("drequeue exited 0, but of %d lines %d were in the rotated queue file and %d in dqueue/peer: %d lost",
			first+second, sent, queued, first+second-sent-queued)
	}
}

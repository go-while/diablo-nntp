package e2e

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestKPDBSameValueNoWrite: KPDBWrite() of the value a field already has writes nothing.
// dreaderd's AssignArticleNo() rewrites NE for every group of every article; when it
// shares dactive.kp with diablo, whose GenerateXRef() already stored that number, the
// write never changed a byte but cost a pwrite() (and dirtied the file) per group.  The
// test sets the file's mtime into the past: a same-value dkp -w must leave the file's
// bytes and mtime alone (on main the pwrite() moved the mtime, and the close stamped a new
// modification time into the header).  A new value of the same length still gets written.
func TestKPDBSameValueNoWrite(t *testing.T) {
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
	run("-w", db, "e2e.test", "NE=0000000005")
	run("-w", db, "e2e.other", "NE=0000000001")

	past := time.Date(2001, 1, 1, 0, 0, 0, 0, time.UTC)
	setPast := func() []byte {
		t.Helper()
		if err := os.Chtimes(db, past, past); err != nil {
			t.Fatal(err)
		}
		data, err := os.ReadFile(db)
		if err != nil {
			t.Fatal(err)
		}
		return data
	}
	mtime := func() time.Time {
		t.Helper()
		fi, err := os.Stat(db)
		if err != nil {
			t.Fatal(err)
		}
		return fi.ModTime()
	}

	before := setPast()
	run("-w", db, "e2e.test", "NE=0000000005")
	after, err := os.ReadFile(db)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Errorf("same value: the file changed:\nbefore %q\nafter  %q", before, after)
	}
	if m := mtime(); !m.Equal(past) {
		t.Errorf("same value: mtime %v, want it left at %v (nothing written)", m, past)
	}

	// The control: a different value of the same length is written in place.
	setPast()
	run("-w", db, "e2e.test", "NE=0000000006")
	live, _, _ := kpRecords(t, db, "e2e.test")
	if len(live) != 1 || !strings.Contains(live[0], " NE=0000000006") {
		t.Errorf("new value: live records %q, want one with NE=0000000006", live)
	}
	if m := mtime(); m.Equal(past) {
		t.Errorf("new value: mtime still %v, the write did not reach the file", past)
	}
}

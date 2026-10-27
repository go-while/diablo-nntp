package e2e

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func dkpRun(t *testing.T, stdin string, args ...string) string {
	t.Helper()
	cmd := exec.Command(filepath.Join(bin(t), "dkp"), args...)
	cmd.Stdin = strings.NewReader(stdin)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Logf("dkp %s: %v", strings.Join(args, " "), err)
	}
	return string(out)
}

// TestKPDBTornTail (D-80): after a crash a KP file can end in a torn record (no newline) or, with
// delayed allocation, in a block of NUL bytes. KPDBWrite() appends at the end of the file
// without looking at what is there, so the new record is glued to the debris: it is
// reported written (dkp exits 0) but no lookup finds it ever again (the line it is on does
// not start with '+'), and the next sort writes its offsets into the debris line.
func TestKPDBTornTail(t *testing.T) {
	for _, tc := range []struct {
		name string
		tail func(b []byte) []byte
	}{
		{"torn record", func(b []byte) []byte { return append(b, "+00000000.0000:cc x"...) }},
		{"NUL block", func(b []byte) []byte { return append(b, make([]byte, 4096)...) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := filepath.Join(t.TempDir(), "kp.db")
			dkpRun(t, "", "-w", db, "aa", "x=1")
			dkpRun(t, "", "-w", db, "bb", "x=1")
			b, err := os.ReadFile(db)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(db, tc.tail(b), 0o644); err != nil {
				t.Fatal(err)
			}
			dkpRun(t, "", "-w", db, "ee", "x=1")
			if out := dkpRun(t, "", "-r", db, "ee"); !strings.Contains(out, ":ee x=1") {
				t.Errorf("record ee written after the damaged tail is not found: %q", out)
			}
			if out := dkpRun(t, "", "-r", db, "bb"); !strings.Contains(out, ":bb x=1") {
				t.Errorf("record bb: %q", out)
			}
		})
	}
}

// TestKPDBTornUpdateDoesNotShadow: a crash in the middle of updating a record leaves a
// half-written copy of it at the end of the file, while the intact old record is still live (it
// is only marked deleted after the new one is complete). The tail repair ends that line with a
// newline; the half-written copy must then be marked deleted too, or the lookup, which scans the
// unsorted records from the end, returns it instead of the intact one.
func TestKPDBTornUpdateDoesNotShadow(t *testing.T) {
	db := filepath.Join(t.TempDir(), "kp.db")
	dkpRun(t, "", "-w", db, "aa", "NE=00001234")
	b, err := os.ReadFile(db)
	if err != nil {
		t.Fatal(err)
	}
	// A torn update of aa: the value is cut off after two digits.
	if err := os.WriteFile(db, append(b, "+00000000.0000:aa NE=00"...), 0o644); err != nil {
		t.Fatal(err)
	}
	dkpRun(t, "", "-w", db, "ee", "x=1")
	if out := dkpRun(t, "", "-r", db, "aa"); !strings.Contains(out, "NE=00001234") {
		t.Errorf("record aa after a torn update and the tail repair: %q, want the intact NE=00001234", out)
	}
	if out := dkpRun(t, "", "-r", db, "ee"); !strings.Contains(out, ":ee x=1") {
		t.Errorf("record ee: %q", out)
	}
}

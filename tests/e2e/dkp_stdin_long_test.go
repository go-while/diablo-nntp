package e2e

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestDKPStdinLongLine (D-128): dkp -w, -r and -x -w read their records from stdin one per
// line, and a line of any length is one record. With fgets() into a 1024-byte buffer a long
// line was cut: its value was written truncated, and the rest of the line was read as a line
// of its own (dkp -r printed a not-found line for it). A last line without a newline counts.
func TestDKPStdinLongLine(t *testing.T) {
	dkp := filepath.Join(bin(t), "dkp")
	for _, size := range []int{16, 1000, 1023, 1024, 2048, 8192} {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			db := filepath.Join(t.TempDir(), "test.kp")
			run := func(stdin string, args ...string) string {
				t.Helper()
				cmd := exec.Command(dkp, args...)
				cmd.Stdin = strings.NewReader(stdin)
				out, err := cmd.CombinedOutput()
				if err != nil {
					t.Fatalf("dkp %v: %v\n%s", args, err, out)
				}
				return string(out)
			}
			key := "long" + strings.Repeat("k", size/2)
			value := strings.Repeat("v", size) + "END"
			run("first a=one\n"+key+" x="+value+"\nsecond b=two\nlast c=three", "-w", db)

			if out := run("", "-r", db, key); !strings.Contains(out, "x="+value) {
				t.Errorf("the long value was not stored whole: %.200q", out)
			}
			for k, v := range map[string]string{"first": "a=one", "second": "b=two", "last": "c=three"} {
				if out := run("", "-r", db, k); !strings.Contains(out, v) {
					t.Errorf("record %s: %q, want %s", k, out, v)
				}
			}

			out := run(key+"\nsecond\n", "-r", db)
			lines := strings.Split(strings.TrimSuffix(out, "\n"), "\n")
			if len(lines) != 2 || !strings.HasPrefix(lines[0], "key "+key+" data") ||
				!strings.Contains(lines[0], "x="+value) || !strings.HasPrefix(lines[1], "key second data") {
				t.Errorf("dkp -r from stdin: want one line per key, got %d lines: %.300q", len(lines), out)
			}

			run(key+"\n", "-x", "-w", db)
			if live, _, _ := kpRecords(t, db, key); len(live) != 0 {
				t.Errorf("the long key is still live after dkp -x from stdin: %d records", len(live))
			}
			if live, _, _ := kpRecords(t, db, "second"); len(live) != 1 {
				t.Errorf("record second: %d live records after deleting the long key, want 1", len(live))
			}
		})
	}
}

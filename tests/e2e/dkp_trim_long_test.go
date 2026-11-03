package e2e

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// A deleted record can span several fgets buffers. All its pieces must be
// discarded, while the live records on either side remain intact.
func TestDKPTrimLongDeletedRecord(t *testing.T) {
	dkp := filepath.Join(bin(t), "dkp")
	for _, size := range []int{16, 1000, 1023, 1024, 2048, 4096} {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			db := filepath.Join(t.TempDir(), "active.kp")
			run := func(args ...string) {
				t.Helper()
				if out, err := exec.Command(dkp, args...).CombinedOutput(); err != nil {
					t.Fatalf("dkp: %v\n%s", err, out)
				}
			}
			run("-w", db, "before", "x=retained-before")
			run("-w", db, "deleted", "x="+strings.Repeat("x", size)+"deleted-tail-marker")
			run("-x", db, "deleted")
			run("-w", db, "after", "x=retained-after")
			run("-w", db, "longlive", "x="+strings.Repeat("y", 4096)+"retained-long-tail")
			run("-t", db)
			data, err := os.ReadFile(db)
			if err != nil {
				t.Fatal(err)
			}
			if bytes.Contains(data, []byte("deleted-tail-marker")) {
				t.Errorf("deleted record's tail remains after trim")
			}
			if !bytes.Contains(data, []byte("retained-long-tail")) {
				t.Error("long live record's tail disappeared")
			}
			for _, key := range []string{"before", "after"} {
				live, _, _ := kpRecords(t, db, key)
				if len(live) != 1 || !strings.Contains(live[0], "retained-"+key) {
					t.Errorf("live record %s changed or disappeared: %q", key, live)
				}
			}
		})
	}
}

// A NUL byte (a torn write) inside a record must neither end it nor cut it
// short: a deleted record with a NUL is dropped without taking the next live
// record with it, and a live record with a NUL is kept as it is instead of
// losing its newline and running into the next line.
func TestDKPTrimNULByte(t *testing.T) {
	dkp := filepath.Join(bin(t), "dkp")
	for _, tc := range []struct{ name, key, old, torn string }{
		{"deleted", "gone", "gone-deleted", "gone\x00deleted"},
		{"live", "torn", "torn-live", "torn\x00live"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := filepath.Join(t.TempDir(), "active.kp")
			run := func(args ...string) {
				t.Helper()
				if out, err := exec.Command(dkp, args...).CombinedOutput(); err != nil {
					t.Fatalf("dkp: %v\n%s", err, out)
				}
			}
			run("-w", db, "first", "x=first-live")
			run("-w", db, "torn", "x=torn-live")
			run("-w", db, "gone", "x=gone-deleted")
			run("-w", db, "next", "x=next-live")
			run("-x", db, "gone")
			data, err := os.ReadFile(db)
			if err != nil {
				t.Fatal(err)
			}
			data = bytes.Replace(data, []byte(tc.old), []byte(tc.torn), 1)
			if err := os.WriteFile(db, data, 0o640); err != nil {
				t.Fatal(err)
			}
			run("-t", db)
			data, err = os.ReadFile(db)
			if err != nil {
				t.Fatal(err)
			}
			if bytes.Contains(data, []byte("deleted")) {
				t.Errorf("deleted record remains after trim: %q", data)
			}
			want := map[string]string{"first": ":first x=first-live\n", "torn": ":torn x=torn-live\n", "next": ":next x=next-live\n"}
			if tc.name == "live" {
				want["torn"] = ":torn x=torn\x00live\n"
			}
			for key, rec := range want {
				if n := bytes.Count(data, []byte(rec)); n != 1 {
					t.Errorf("live record %s occurs %d times after trim, want once: %q", key, n, data)
				}
			}
		})
	}
}

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

package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestKPDBBlankLine (D-79): a blank or very short line in a KP database (a hand edit, or the
// debris of a crash) corrupts the sort: KPDBSort() writes the 8 hex digits of a record's
// offset at line start + 1 whatever the line's length, over the following line, and a
// lookup then recurses until the stack is gone (dkp dies with SIGSEGV).
func TestKPDBBlankLine(t *testing.T) {
	db := filepath.Join(t.TempDir(), "kp.db")
	for _, k := range []string{"aa", "bb", "cc", "dd"} {
		dkpRun(t, "", "-w", db, k, "x=1")
	}
	b, err := os.ReadFile(db)
	if err != nil {
		t.Fatal(err)
	}
	// a blank line before the last record
	i := strings.LastIndex(strings.TrimRight(string(b), "\n"), "\n") + 1
	if err := os.WriteFile(db, []byte(string(b[:i])+"\n"+string(b[i:])), 0o644); err != nil {
		t.Fatal(err)
	}

	done := make(chan string, 1)
	go func() {
		var res []string
		for _, k := range []string{"aa", "bb", "cc", "dd"} {
			if out := dkpRun(t, "", "-r", db, k); !strings.Contains(out, ":"+k+" x=1") {
				res = append(res, k+": "+strings.TrimSpace(out))
			}
		}
		done <- strings.Join(res, "; ")
	}()
	select {
	case bad := <-done:
		if bad != "" {
			t.Errorf("dkp -r after a blank line was added to the KP file did not find: %s", bad)
		}
	case <-time.After(60 * time.Second):
		t.Fatal("dkp -r did not return within 60s")
	}
}

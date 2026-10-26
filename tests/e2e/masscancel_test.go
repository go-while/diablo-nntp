package e2e

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestMasscancelLastLine: dgrpctl masscancel reads every Message-ID on stdin, also a last
// line without a newline. Two invalid IDs are given, so both must be reported. Before D-17's
// fix its while (!feof()) loop dropped a last line that fgets() ended at EOF.
func TestMasscancelLastLine(t *testing.T) {
	s := NewReaderSite(t, ReaderOptions{SpoolAddr: "127.0.0.1:1", NoStart: true})
	cmd := exec.Command(filepath.Join(s.Bin, "dgrpctl"), "masscancel", "e2e.test")
	cmd.Dir = s.Dir
	cmd.Env = append(os.Environ(), s.Env...)
	cmd.Stdin = strings.NewReader("bogus1\nbogus2")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("dgrpctl masscancel: %v\n%s", err, out)
	}
	for _, id := range []string{"bogus1", "bogus2"} {
		if !strings.Contains(string(out), "invalid message-id: "+id) {
			t.Errorf("%s not read; output:\n%s", id, out)
		}
	}
}

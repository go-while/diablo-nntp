package e2e

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-while/nntp-testkit/client"
)

// TestDeepRevDidumpRecordSize (L7-13): didump took the record size from the history header
// as it is. With 0 it dumped nothing, with a size smaller than a record it printed pieces of
// records as entries (and read past the last one), both with exit status 0; with a larger
// one, -t read that many bytes into the 28-byte record on the stack. A history with such a
// header (a damaged or foreign file) is now refused as corrupted, as dhisexpire does.
func TestDeepRevDidumpRecordSize(t *testing.T) {
	s := NewSite(t, SiteOptions{})
	p := s.Peer(t)
	id := msgID(t)
	p.ExpectCmd(335, "IHAVE %s", id)
	p.MustSend(client.Stuff(newArticle(id, "e2e.test", "history", "body")))
	p.ExpectStatus(235, "IHAVE")
	p.ExpectCmd(205, "QUIT")
	h, err := os.ReadFile(filepath.Join(s.Dir, "dhistory"))
	if err != nil || len(h) < 16 {
		t.Fatalf("dhistory: %v (%d bytes)", err, len(h))
	}
	for _, tc := range []struct {
		size uint16
		args []string
	}{
		{0, nil},
		{4, nil},
		{65535, []string{"-t"}},
	} {
		damaged := append([]byte(nil), h...)
		// HistHead: hmagic, hashSize (4 bytes each), version, henSize (2 bytes each)
		damaged[10], damaged[11] = byte(tc.size), byte(tc.size>>8)
		path := filepath.Join(t.TempDir(), "dhistory")
		if err := os.WriteFile(path, damaged, 0o644); err != nil {
			t.Fatal(err)
		}
		out, err := s.TryRun("didump", append(tc.args, path)...)
		if len(out) > 2000 {
			out = out[:2000] + " ..."
		}
		var ee *exec.ExitError
		if !errors.As(err, &ee) || ee.ExitCode() != 1 {
			t.Errorf("record size %d, didump %v: %v, want exit status 1\n%s", tc.size, tc.args, err, out)
		}
		if !strings.Contains(out, "record size") {
			t.Errorf("record size %d, didump %v did not report the record size:\n%s", tc.size, tc.args, out)
		}
	}
	// the undamaged history still dumps the article
	out, err := s.TryRun("didump", filepath.Join(s.Dir, "dhistory"))
	if err != nil || !strings.Contains(out, "DUMP ") {
		t.Errorf("didump of the good history: %v\n%.2000s", err, out)
	}
}

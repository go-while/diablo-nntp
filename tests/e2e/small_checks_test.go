package e2e

import (
	"errors"
	"github.com/go-while/nntp-testkit/client"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestDexpireOverListUnwritable (D-105): dexpire -o writes the hashes of the expired articles
// to the dexpover list. When that file cannot be opened it says so and expires anyway. Before,
// it wrote to and closed the NULL FILE pointer and crashed, so the spool was not expired.
func TestDexpireOverListUnwritable(t *testing.T) {
	s := NewSite(t, SiteOptions{
		Config: map[string]string{"path_dexpover_list": "%s/missing-dir/dexpover.dat"},
		SpoolCtl: `spool 00
    minfree 1m
    keeptime 1
end
metaspool trad
    spool 00
end
expire * trad
`})
	id := msgID(t)
	p := s.Peer(t)
	p.ExpectCmd(335, "IHAVE %s", id)
	p.MustSend(client.Stuff(newArticle(id, "e2e.test", "to be expired", "old body")))
	p.ExpectStatus(235, "IHAVE "+id)
	p.ExpectCmd(205, "QUIT")
	dirs, _ := filepath.Glob(filepath.Join(s.Dir, "spool", "news", "D.*"))
	if len(dirs) == 0 {
		t.Fatalf("no spool directory after storing %s", id)
	}
	time.Sleep(2 * time.Second) // past keeptime 1 (seconds)
	newerSpoolDir(t, s)

	out, err := s.TryRun("dexpire", "-a", "-o", "-v")
	if err != nil {
		t.Fatalf("dexpire -a -o with an unwritable list: %v\n%s", err, out)
	}
	if !strings.Contains(out, "Error opening") {
		t.Errorf("dexpire did not report the list it could not open:\n%s", out)
	}
	for _, d := range dirs {
		if _, err := os.Stat(d); err == nil {
			t.Errorf("spool directory %s still there after dexpire -a -o\n%s", d, out)
		}
	}
}

// TestDhisexpireBadRecordSize (D-104): a history whose header gives a record size of 0, or one
// smaller than a record, is refused as corrupted. Before, dhisexpire divided by the record
// size (SIGFPE on 0) or read past each record.
func TestDhisexpireBadRecordSize(t *testing.T) {
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
	for _, size := range []uint16{0, 4} {
		damaged := append([]byte(nil), h...)
		// HistHead: hmagic, hashSize (4 bytes each), version, henSize (2 bytes each)
		damaged[10], damaged[11] = byte(size), byte(size>>8)
		path := filepath.Join(t.TempDir(), "dhistory")
		if err := os.WriteFile(path, damaged, 0o644); err != nil {
			t.Fatal(err)
		}
		out, err := s.TryRun("dhisexpire", path)
		var ee *exec.ExitError
		if !errors.As(err, &ee) || ee.ExitCode() != 1 {
			t.Errorf("record size %d: dhisexpire %v, want exit status 1\n%s", size, err, out)
		}
		if !strings.Contains(out, "record size") {
			t.Errorf("record size %d: dhisexpire did not report the record size:\n%s", size, out)
		}
	}
}

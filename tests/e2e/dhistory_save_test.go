package e2e

// path_dhistory_save: a directory for the copies of the history that the adm scripts make
// (dumphist's snapshots, biweekly.atrim's dhisexpire backup), so that a site with its history on
// a RAM disk keeps them on a disk; and dhisexpire -m, which links the new history into place
// when it is on the history's own filesystem instead of copying it.

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestDhistorySaveDumphist: with path_dhistory_save, dpath prints it and dumphist keeps
// dhistory.snp and dhistory.old in that directory, none next to the history. Fails on main:
// dpath does not know dhistory_save, and the snapshots are made next to the history.
func TestDhistorySaveDumphist(t *testing.T) {
	save := t.TempDir()
	s := NewSite(t, SiteOptions{NoStart: true, Config: map[string]string{"path_dhistory_save": save}})
	if out, err := s.TryRun("dpath", "dhistory_save"); err != nil || strings.TrimSpace(out) != save {
		t.Errorf("dpath dhistory_save: %q, %v; want %s", out, err, save)
	}

	hist := filepath.Join(s.Dir, "dhistory")
	s.WriteFile(t, "dhistory", "first")
	if out, code := admScript(t, s, "dumphist"); code != 0 {
		t.Fatalf("dumphist: exit %d\n%s", code, out)
	}
	s.WriteFile(t, "dhistory", "second")
	if out, code := admScript(t, s, "dumphist"); code != 0 {
		t.Fatalf("second dumphist: exit %d\n%s", code, out)
	}
	snp, _ := os.ReadFile(filepath.Join(save, "dhistory.snp"))
	old, _ := os.ReadFile(filepath.Join(save, "dhistory.old"))
	if string(snp) != "second" || string(old) != "first" {
		t.Errorf("%s/dhistory.snp %q, dhistory.old %q; want second and first", save, snp, old)
	}
	for _, name := range []string{hist + ".snp", hist + ".old"} {
		if _, err := os.Stat(name); err == nil {
			t.Errorf("%s made next to the history", name)
		}
	}
	for _, dir := range []string{s.Dir, save} {
		if left, _ := filepath.Glob(filepath.Join(dir, "dhistory.tmp.*")); len(left) != 0 {
			t.Errorf("temporary files left: %v", left)
		}
	}
}

// TestDhistorySaveDumphistNoDir: a path_dhistory_save that is not a directory makes dumphist
// fail and leaves no snapshot next to the history. Fails on main: the snapshot is made next to
// the history and dumphist exits 0.
func TestDhistorySaveDumphistNoDir(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing")
	s := NewSite(t, SiteOptions{NoStart: true, Config: map[string]string{"path_dhistory_save": missing}})
	s.WriteFile(t, "dhistory", "first")
	out, code := admScript(t, s, "dumphist")
	if code == 0 || !strings.Contains(out, "not a directory") {
		t.Errorf("dumphist with a missing path_dhistory_save: exit %d, want a failure that names it\n%s", code, out)
	}
	if _, err := os.Stat(filepath.Join(s.Dir, "dhistory.snp")); err == nil {
		t.Errorf("dhistory.snp made next to the history")
	}
}

// TestDhistorySaveBiweeklyAtrim: with path_dhistory_save, biweekly.atrim keeps dhisexpire's
// backup of the old history there and removes a backup left next to the history; the article
// stays in the new history. Fails on main: the backup is dhistory.bak next to the history.
func TestDhistorySaveBiweeklyAtrim(t *testing.T) {
	save := t.TempDir()
	s := NewSite(t, SiteOptions{Setup: withAdm(admSource(t)), Config: map[string]string{"path_dhistory_save": save}})
	id := s.Inject(t, "e2e.test", 1)[0]
	s.WriteFile(t, "dhistory.bak", "a backup from before path_dhistory_save")
	runAdm(t, s, "biweekly.atrim")

	b, err := os.ReadFile(filepath.Join(save, "dhistory.bak"))
	if err != nil || len(b) < 100 || !bytes.Equal(b[:4], deadMagic) {
		t.Errorf("%s/dhistory.bak: %d bytes, %v; want the old history (marked dead)", save, len(b), err)
	}
	for _, name := range []string{"dhistory.bak", "dhistory.new", "dhistory.tmp"} {
		if _, err := os.Stat(filepath.Join(s.Dir, name)); err == nil {
			t.Errorf("%s left next to the history", name)
		}
	}
	p := s.Peer(t)
	p.ExpectCmd(435, "IHAVE %s", id) // still in history
	p.ExpectCmd(223, "STAT %s", id)
	p.ExpectCmd(205, "QUIT")
}

// TestDhisexpireMLinksOnSameFilesystem: dhisexpire -a -m with the new history on the history's
// own filesystem links it into place instead of copying it (a copy is a third history-sized
// file, on a RAM disk in memory), and diablo still knows the article. Fails on main: -m always
// copied, and said nothing about it.
func TestDhisexpireMLinksOnSameFilesystem(t *testing.T) {
	s := NewSite(t, SiteOptions{})
	id := s.Inject(t, "e2e.test", 1)[0]
	out, err := s.TryRun("dhisexpire", "-a", "-m", "-o", "0", s.Dir+"/dhistory")
	if err != nil {
		t.Fatalf("dhisexpire -a -m: %v\n%s", err, out)
	}
	if !strings.Contains(out, "New history linked to") {
		t.Errorf("dhisexpire -a -m on one filesystem: want \"New history linked to\"\n%s", out)
	}
	for _, name := range []string{"dhistory.new", "dhistory.tmp"} {
		if _, err := os.Stat(filepath.Join(s.Dir, name)); err == nil {
			t.Errorf("%s left behind", name)
		}
	}
	p := s.Peer(t)
	p.ExpectCmd(438, "CHECK %s", id)
	p.ExpectCmd(205, "QUIT")
}

// TestDiabloCtlSnapshotInSaveDir: without a history file, diablo-ctl check notes a snapshot in
// path_dhistory_save. Fails on main: diablo-ctl only looks next to the history.
func TestDiabloCtlSnapshotInSaveDir(t *testing.T) {
	save := t.TempDir()
	s, run := ctlSite(t, map[string]string{"path_dhistory_save": save}, "")
	skipWithoutPreflight(t, s, run)
	os.Remove(filepath.Join(s.Dir, "dhistory"))
	if err := os.WriteFile(filepath.Join(save, "dhistory.snp"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, code := run("check")
	wantCtl(t, out, code, 0, "check with a snapshot in path_dhistory_save")
	if !strings.Contains(out, "note:") || !strings.Contains(out, save) {
		t.Errorf("check: %q, want a note naming %s", out, save)
	}
}

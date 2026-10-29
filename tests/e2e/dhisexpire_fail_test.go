package e2e

import (
	"bytes"
	"net"
	"os"
	"strings"
	"testing"
	"time"
)

// deadMagic is HDEADMAGIC (0xDEADF5E6) as it is written at the start of dhistory.
var deadMagic = []byte{0xe6, 0xf5, 0xad, 0xde}

// TestDhisexpireFailureKeepsHistoryAlive (D-57): dhisexpire marks the live history dead for the
// last pass and then makes the backup and installs the new file. When that fails (here: the
// backup directory does not exist) it must mark the history alive again, so that diablo keeps
// accepting connections. Fails on main: the history stayed dead and every new connection
// ended without a greeting.
func TestDhisexpireFailureKeepsHistoryAlive(t *testing.T) {
	s := NewSite(t, SiteOptions{})
	p := s.Peer(t)
	id := msgID(t)
	expect(t, p, 335, "IHAVE %s", id)
	send(t, p, wire(newArticle(id, "e2e.test", "history", "body")))
	status(t, p, 235, "IHAVE")
	expect(t, p, 205, "QUIT")

	out, err := s.TryRun("dhisexpire", "-a", "-o", s.Dir+"/missing-dir/dhistory.bak", s.Dir+"/dhistory")
	if err == nil {
		t.Fatalf("dhisexpire succeeded with an unusable backup path:\n%s", out)
	}
	if !strings.Contains(out, "keeping old history") {
		t.Errorf("dhisexpire output: %s", out)
	}
	h, rerr := os.ReadFile(s.Dir + "/dhistory")
	if rerr != nil || len(h) < 8 {
		t.Fatalf("dhistory: %v (%d bytes)", rerr, len(h))
	}
	if bytes.Equal(h[:4], deadMagic) {
		t.Errorf("dhistory is still marked dead after the failure")
	}

	d := net.Dialer{LocalAddr: &net.TCPAddr{IP: net.ParseIP(PeerIP)}, Timeout: 3 * time.Second}
	nc, derr := d.Dial("tcp", s.Addr)
	if derr != nil {
		t.Fatal(derr)
	}
	defer nc.Close()
	nc.SetReadDeadline(time.Now().Add(5 * time.Second))
	buf := make([]byte, 200)
	n, rerr := nc.Read(buf)
	if rerr != nil || !strings.HasPrefix(string(buf[:n]), "20") {
		t.Errorf("no greeting after the failed dhisexpire: %q, %v", buf[:n], rerr)
	}
}

// TestDhisexpireKeepsBackup: the normal run still works after the change to the backup step:
// the new history is installed, the backup holds the old file (an earlier backup is replaced by
// it), no temporary name is left, and diablo still knows the article.
func TestDhisexpireKeepsBackup(t *testing.T) {
	s := NewSite(t, SiteOptions{})
	p := s.Peer(t)
	id := msgID(t)
	expect(t, p, 335, "IHAVE %s", id)
	send(t, p, wire(newArticle(id, "e2e.test", "history", "body")))
	status(t, p, 235, "IHAVE")
	expect(t, p, 205, "QUIT")

	bak := s.Dir + "/dhistory.bak"
	if err := os.WriteFile(bak, []byte("an earlier backup"), 0o644); err != nil {
		t.Fatal(err)
	}
	if out, err := s.TryRun("dhisexpire", "-a", "-o", bak, s.Dir+"/dhistory"); err != nil {
		t.Fatalf("dhisexpire: %v\n%s", err, out)
	}
	// The backup is the old history (still carrying its dead mark, as it always did).
	if b, err := os.ReadFile(bak); err != nil || len(b) < 100 || strings.HasPrefix(string(b), "an earlier") {
		t.Errorf("backup: %d bytes, err %v: the earlier backup was not replaced by the old history", len(b), err)
	}
	if _, err := os.Stat(bak + ".new"); err == nil {
		t.Errorf("temporary backup name left behind")
	}
	p = s.Peer(t)
	expect(t, p, 438, "CHECK %s", id)
	expect(t, p, 205, "QUIT")
}

// TestDhisexpireNoteWithoutA (D-77): without -a the old history stays marked dead and the new
// one is left beside it; the run says so at the end, and dhisexpire -u makes the old one usable.
// Fails on the first version of the note, which could never print.
func TestDhisexpireNoteWithoutA(t *testing.T) {
	s := NewSite(t, SiteOptions{})
	p := s.Peer(t)
	id := msgID(t)
	expect(t, p, 335, "IHAVE %s", id)
	send(t, p, wire(newArticle(id, "e2e.test", "history", "body")))
	status(t, p, 235, "IHAVE")
	expect(t, p, 205, "QUIT")

	out, err := s.TryRun("dhisexpire", s.Dir+"/dhistory", s.Dir+"/dhistory.new")
	if err != nil {
		t.Fatalf("dhisexpire: %v\n%s", err, out)
	}
	if !strings.Contains(out, "stays marked dead") {
		t.Errorf("no note that the old history stays dead:\n%s", out)
	}
	h, rerr := os.ReadFile(s.Dir + "/dhistory")
	if rerr != nil || len(h) < 8 || !bytes.Equal(h[:4], deadMagic) {
		t.Errorf("dhistory is not marked dead after a run without -a (%v)", rerr)
	}
	if out, err := s.TryRun("dhisexpire", "-u", s.Dir+"/dhistory"); err != nil {
		t.Fatalf("dhisexpire -u: %v\n%s", err, out)
	}
	h, rerr = os.ReadFile(s.Dir + "/dhistory")
	if rerr != nil || len(h) < 8 || bytes.Equal(h[:4], deadMagic) {
		t.Errorf("dhistory is still marked dead after dhisexpire -u (%v)", rerr)
	}
}

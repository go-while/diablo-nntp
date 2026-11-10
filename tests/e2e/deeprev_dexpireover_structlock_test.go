package e2e

import (
	"bytes"
	"context"
	"encoding/binary"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

// TestDeepRevDexpireoverStructLock (second review of L7-09, Sol F3): a resize or a rewrite of an
// over file holds the lock at offset 4 exclusive while it copies the slots and writes them back;
// dreaderd takes that lock shared before it reads the header and maps the slots. Plain expiry
// (-y, -e, -o) took only the slot lock at offset 0, so it did not wait for a resize in another
// dexpireover: it could mark a slot expired that the resize then wrote back from its copy, or
// work on a map that the resize had just made shorter.
//
// Expiry now takes offset 4 shared first. The test holds it exclusive (as a resize does) and
// runs dexpireover -y on a group whose only article is past its expiry: dexpireover must wait,
// leave the slot alone while the lock is held, and expire it once the lock is free.
func TestDeepRevDexpireoverStructLock(t *testing.T) {
	s, home := overSite(t, map[string]string{"readergrouphash": "hierarchy"})
	s.Run(t, "dgrpctl", "adjust", "e2e.test", "NE", "=1")
	s.WriteFile(t, "dexpire.ctl", "e2e.*:x30\n")
	over := filepath.Join(home, "e2e", "o.0.e2e.test")
	o := binary.NativeEndian
	head := overHead("e2e.test")
	index := append(head, make([]byte, 1000*32)...)
	slot := len(head) + 32 // article 1
	o.PutUint32(index[slot:], 1)
	o.PutUint32(index[slot+8:], 10)
	o.PutUint32(index[slot+12:], 0x952a1fae)
	o.PutUint32(index[slot+16:], 0x54dcbdde)
	o.PutUint32(index[slot+20:], 1000)
	o.PutUint32(index[slot+24:], uint32(time.Now().Add(-60*24*time.Hour).Unix())) // 60 days old
	writeOver(t, over, index)

	f, err := os.OpenFile(over, os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	// Read through f: closing any other descriptor of the file would drop the lock this
	// process holds on it.
	artNo := func() int32 {
		b := make([]byte, 4)
		if _, err := f.ReadAt(b, int64(slot)); err != nil {
			t.Fatalf("read %s: %v", over, err)
		}
		return int32(o.Uint32(b))
	}
	lk := syscall.Flock_t{Type: syscall.F_WRLCK, Whence: 0, Start: 4, Len: 4}
	if err := syscall.FcntlFlock(f.Fd(), syscall.F_SETLK, &lk); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	// -l60: the lock wait gives up after 1 s by default
	cmd := exec.CommandContext(ctx, filepath.Join(s.Bin, "dexpireover"), "-y", "-v", "-l60")
	cmd.Dir = s.Dir
	cmd.Env = append(os.Environ(), s.Env...)
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	if err := waitLockWaiter(cmd.Process.Pid, done); err != nil {
		t.Fatalf("dexpireover -y did not wait for the lock at offset 4 (%v); article number in the slot now %d\n%s",
			err, artNo(), out.String())
	}
	if n := artNo(); n != 1 {
		t.Errorf("the slot was changed (article number %d) while the lock at offset 4 was held", n)
	}

	lk.Type = syscall.F_UNLCK
	if err := syscall.FcntlFlock(f.Fd(), syscall.F_SETLK, &lk); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatalf("dexpireover -y: %v\n%s", err, out.String())
	}
	if n := artNo(); n != -2 {
		t.Errorf("after the lock was freed the article was not expired: article number %d, want -2\n%s", n, out.String())
	}
}

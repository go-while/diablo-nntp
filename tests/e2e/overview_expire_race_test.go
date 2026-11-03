package e2e

import (
	"bytes"
	"context"
	"encoding/binary"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"
)

// A writer can reuse a ring slot after the expiration snapshot was made.
func TestOpenBSDExpireDoesNotOverwriteNewOverview(t *testing.T) {
	if runtime.GOOS != "openbsd" {
		t.Skip("OpenBSD overview snapshot")
	}
	s, home := overSite(t, map[string]string{"readergrouphash": "hierarchy"})
	s.WriteFile(t, "dexpire.ctl", "*:x1\n")
	s.Run(t, "dgrpctl", "adjust", "e2e.test", "NE", "=1")
	head := overHead("e2e.test")
	index := append(head, make([]byte, 1000*32)...)
	slot := index[len(head)+32 : len(head)+64]
	binary.NativeEndian.PutUint32(slot, 1)
	binary.NativeEndian.PutUint32(slot[24:], uint32(time.Now().Add(-48*time.Hour).Unix()))
	path := filepath.Join(home, "e2e", "o.0.e2e.test")
	writeOver(t, path, index)
	f, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	lockRange(t, f, syscall.F_WRLCK, 0, 4)
	defer lockRange(t, f, syscall.F_UNLCK, 0, 4)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, filepath.Join(s.Bin, "dexpireover"), "-y", "-v")
	cmd.Dir = s.Dir
	cmd.Env = append(os.Environ(), s.Env...)
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		t.Fatalf("expiration did not wait on the index lock: %v\n%s", err, out.String())
	case <-time.After(time.Second):
	}
	newSlot := append([]byte(nil), slot...)
	binary.NativeEndian.PutUint32(newSlot, 1001) // same ring slot, newer article
	binary.NativeEndian.PutUint32(newSlot[24:], uint32(time.Now().Unix()))
	if _, err := f.WriteAt(newSlot, int64(len(head)+32)); err != nil {
		t.Fatal(err)
	}
	lockRange(t, f, syscall.F_UNLCK, 0, 4)
	if err := <-done; err != nil {
		t.Fatalf("dexpireover: %v\n%s", err, out.String())
	}
	got := make([]byte, len(newSlot))
	if _, err := f.ReadAt(got, int64(len(head)+32)); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, newSlot) {
		t.Errorf("expiration overwrote the replacement: got %x want %x\n%s", got, newSlot, out.String())
	}
}

func TestOpenBSDExpireRefreshesAfterOwnWrite(t *testing.T) {
	if runtime.GOOS != "openbsd" {
		t.Skip("OpenBSD overview snapshot")
	}
	s, home := overSite(t, map[string]string{"readergrouphash": "hierarchy"})
	s.WriteFile(t, "dexpire.ctl", "*:x1\n")
	s.Run(t, "dgrpctl", "adjust", "e2e.test", "NE", "=1")
	head := overHead("e2e.test")
	index := append(head, make([]byte, 1000*32)...)
	slot := index[len(head)+32 : len(head)+64]
	binary.NativeEndian.PutUint32(slot, 1)
	binary.NativeEndian.PutUint32(slot[24:], uint32(time.Now().Add(-48*time.Hour).Unix()))
	writeOver(t, filepath.Join(home, "e2e", "o.0.e2e.test"), index)
	out := s.Run(t, "dexpireover", "-y", "-NB", "-v")
	active := s.Run(t, "dkp", "-r", filepath.Join(s.Dir, "dactive.kp"), "e2e.test")
	if !strings.Contains(active, "NB=0000000002") {
		t.Errorf("NB did not advance after expiration: %s\n%s", active, out)
	}
}

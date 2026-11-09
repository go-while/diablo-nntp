//go:build linux

package e2e

import (
	"net"
	"strings"
	"syscall"
	"testing"
	"time"
)

// A streaming peer can stop sending while it waits for small NNTP replies.
// Disable its quick ACKs and pipeline two commands: Nagle must not hold the
// second reply until the peer's delayed-ACK timer fires (about 40 ms on Linux).
func TestFeederRepliesWithoutDelayedACK(t *testing.T) {
	s := NewSite(t, SiteOptions{})
	var fastest = time.Hour
	for attempt := 0; attempt < 5; attempt++ {
		nc, r := dialPeerRaw(t, s)
		if line, err := greetingWithin(nc, r, 5*time.Second); err != nil || !strings.HasPrefix(line, "200 ") {
			t.Fatalf("greeting: %q, %v", line, err)
		}
		raw, err := nc.(*net.TCPConn).SyscallConn()
		if err != nil {
			t.Fatal(err)
		}
		var sockErr error
		if err := raw.Control(func(fd uintptr) {
			sockErr = syscall.SetsockoptInt(int(fd), syscall.IPPROTO_TCP, syscall.TCP_QUICKACK, 0)
		}); err != nil || sockErr != nil {
			t.Fatalf("disable quick ACK: %v / %v", err, sockErr)
		}
		nc.SetDeadline(time.Now().Add(5 * time.Second))
		start := time.Now()
		if _, err := nc.Write([]byte("MODE STREAM\r\nMODE STREAM\r\n")); err != nil {
			t.Fatal(err)
		}
		for i := 0; i < 2; i++ {
			if line, err := r.ReadString('\n'); err != nil || !strings.HasPrefix(line, "203 ") {
				t.Fatalf("MODE STREAM reply %d: %q, %v", i, line, err)
			}
		}
		elapsed := time.Since(start)
		if elapsed < fastest {
			fastest = elapsed
		}
		nc.Close()
	}
	// Take the fastest of five fresh connections so scheduler delays do not
	// turn this into a test of the CI host's load.
	if fastest >= 30*time.Millisecond {
		t.Fatalf("two pipelined replies took at least %v; waiting for the peer's delayed ACK", fastest)
	}
	t.Logf("fastest pair of pipelined replies: %v", fastest)
}

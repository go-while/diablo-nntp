package e2e

import (
	"bufio"
	"net"
	"strings"
	"syscall"
	"testing"
	"time"

	testkit "github.com/go-while/nntp-testkit"
)

// TestDeepRevSigtermDuringXclient (L1-05): a session process took itself for the master in
// the signal handlers until it had authenticated the peer and, for a %XCLIENT label, read
// the proxy's "xclient" line (DidFork was set after both). A SIGHUP from the master in that
// time (dicmd exit, SIGTERM) ran the master's branch in the child: it sent SIGHUP to the
// pids of the process table it had inherited and went on waiting for the xclient line. With
// a proxy connection that is open and silent the master then waited for that child forever.
//
// SIGTERM to the master with such a connection open: the master must exit.
func TestDeepRevSigtermDuringXclient(t *testing.T) {
	s := NewSite(t, SiteOptions{Newsfeeds: GlobalLabel + "label %XCLIENT\n    inhost " + PeerIP + "\nend\n"})
	deepRevKillChildren(t, s, "diablo")

	d := net.Dialer{LocalAddr: &net.TCPAddr{IP: net.ParseIP(PeerIP)}, Timeout: testkit.Deadline()}
	c, err := d.Dial("tcp", s.Addr)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	_ = c.SetReadDeadline(time.Now().Add(testkit.Deadline()))
	greeting, err := bufio.NewReader(c).ReadString('\n')
	if err != nil || !strings.Contains(greeting, "XCLIENT session ready") {
		t.Fatalf("greeting for the %%XCLIENT label: %q, %v; want \"200 ... XCLIENT session ready\"", greeting, err)
	}
	// The session process now waits for the xclient line.

	if err := syscall.Kill(s.Proc.Pid(), syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	select {
	case <-s.Proc.Done():
	case <-time.After(10 * time.Second):
		t.Fatalf("diablo still running 10s after SIGTERM with a silent XCLIENT connection open")
	}
}

package e2e

// The feeder's real-time stats (feederrtstats label|host) live in the feedstats file:
// a dummy record, then one FeedStats record per label or host, which every diablo
// child and dnewslink maps MAP_SHARED and updates under LockFeedRegion().
//
//	connect   The incoming child counted RecStats.ConnectCnt (and set TimeStart)
//	          without taking the FSTATS_IN lock; it only released it afterwards.
//	per host  LockFeedRegion() locked an offset inside the struct, not inside the
//	          host's record, so every host shared one lock per stats type in the
//	          dummy record.  The record's own region is locked now.
//	snapshot  dfeedinfo -P locked each record with a write lock on a read-only
//	          descriptor, which fails (EBADF), at the record start, where no writer
//	          locks; it took no lock at all.  It now read-locks the whole record.
//
// The test holds fcntl locks on parts of the file from the test process and watches
// what the diablo child or dfeedinfo does meanwhile.  dfeedinfo -i/-s read without
// locks, so the counters can be read while the test holds a lock.  The test must not
// open the feedstats file itself while it holds a lock: closing any descriptor of the
// file would drop the test process's locks.

import (
	"bufio"
	"context"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// feedStatsCon returns the con= count of label in dfeedinfo's -i (in) or -s (spool) output.
func feedStatsCon(t *testing.T, s *Site, flag, label string) int {
	t.Helper()
	out := s.Run(t, "dfeedinfo", flag, "-r", label)
	m := regexp.MustCompile(`(?m)^\S+ ` + regexp.QuoteMeta(label) + ` .* con=(\d+) `).FindStringSubmatch(out)
	if m == nil {
		t.Fatalf("dfeedinfo %s: no line for %s:\n%s", flag, label, out)
	}
	n, _ := strconv.Atoi(m[1])
	return n
}

// lockRange takes a write lock on [start, start+n) of f from the test process.
func lockRange(t *testing.T, f *os.File, typ int16, start, n int64) {
	t.Helper()
	fl := syscall.Flock_t{Type: typ, Whence: 0, Start: start, Len: n}
	if err := syscall.FcntlFlock(f.Fd(), syscall.F_SETLK, &fl); err != nil {
		t.Fatalf("fcntl lock %d+%d type %d: %v", start, n, typ, err)
	}
}

// dialPeerRaw connects from PeerIP without waiting for the greeting.
func dialPeerRaw(t *testing.T, s *Site) (net.Conn, *bufio.Reader) {
	t.Helper()
	d := net.Dialer{LocalAddr: &net.TCPAddr{IP: net.ParseIP(PeerIP)}, Timeout: 10 * time.Second}
	nc, err := d.Dial("tcp", s.Addr)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { nc.Close() })
	return nc, bufio.NewReader(nc)
}

// greetingWithin reads the greeting, giving up after d.
func greetingWithin(nc net.Conn, r *bufio.Reader, d time.Duration) (string, error) {
	nc.SetReadDeadline(time.Now().Add(d))
	defer nc.SetReadDeadline(time.Time{})
	return r.ReadString('\n')
}

func TestFeedStatsHostLock(t *testing.T) {
	s := NewSite(t, SiteOptions{Config: map[string]string{"feederrtstats": "label"}})
	path := filepath.Join(s.Dir, "feedstats")

	// The first session creates the record of label "peer" after the dummy one.
	c := s.Peer(t)
	expect(t, c, 205, "QUIT")
	var size int64
	waitFor(t, 10*time.Second, "the peer's feedstats record", func() bool {
		st, err := os.Stat(path)
		if err == nil {
			size = st.Size()
		}
		return err == nil && size > 0 && size%2 == 0 && feedStatsCon(t, s, "-i", "peer") == 1
	})
	rec := size / 2 // sizeof(FeedStats): two records, the dummy and peer

	f, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	t.Run("connect", func(t *testing.T) {
		// The peer's whole record is locked: the new child must wait before it counts
		// the connection, and before its greeting.
		lockRange(t, f, syscall.F_WRLCK, rec, rec)
		nc, r := dialPeerRaw(t, s)
		_, gerr := greetingWithin(nc, r, 1500*time.Millisecond)
		con := feedStatsCon(t, s, "-i", "peer")
		lockRange(t, f, syscall.F_UNLCK, rec, rec)
		if con != 1 {
			t.Errorf("RecStats.ConnectCnt went to %d while the peer's record was locked, want 1 (counted without the FSTATS_IN lock)", con)
		}
		if gerr == nil {
			t.Errorf("greeting sent while the peer's record was locked: the child took no lock in that record")
		}
		if gerr != nil {
			if g, err := greetingWithin(nc, r, 10*time.Second); err != nil || !strings.HasPrefix(g, "200 ") {
				t.Fatalf("greeting after unlock: %q %v", g, err)
			}
		}
		nc.Write([]byte("QUIT\r\n"))
		waitFor(t, 10*time.Second, "con=2", func() bool { return feedStatsCon(t, s, "-i", "peer") == 2 })
		if n := feedStatsCon(t, s, "-s", "peer"); n != 2 {
			t.Errorf("SpoolStats.ConnectCnt %d, want 2", n)
		}
	})

	t.Run("per host", func(t *testing.T) {
		// The dummy record (but not bytes 0-3, the slot allocation lock) is locked:
		// that is no host's region, so the peer's child goes on.  Before, every host's
		// FSTATS_* lock was in there.
		lockRange(t, f, syscall.F_WRLCK, 4, rec-4)
		nc, r := dialPeerRaw(t, s)
		g, gerr := greetingWithin(nc, r, 5*time.Second)
		if gerr == nil {
			nc.Write([]byte("QUIT\r\n"))
			waitFor(t, 10*time.Second, "spool con=3", func() bool { return feedStatsCon(t, s, "-s", "peer") == 3 })
		}
		lockRange(t, f, syscall.F_UNLCK, 4, rec-4)
		if gerr != nil || !strings.HasPrefix(g, "200 ") {
			t.Fatalf("no greeting while the dummy record was locked (%q %v): the stats locks are not per host", g, gerr)
		}
		if n := feedStatsCon(t, s, "-i", "peer"); n != 3 {
			t.Errorf("RecStats.ConnectCnt %d, want 3", n)
		}
	})

	t.Run("snapshot", func(t *testing.T) {
		// A writer holds the peer's FSTATS_IN lock: the snapshot waits for it.
		lockRange(t, f, syscall.F_WRLCK, rec, rec)
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, filepath.Join(s.Bin, "dfeedinfo"), "-Ptest")
		cmd.Dir = s.Dir
		cmd.Env = append(os.Environ(), s.Env...)
		done := make(chan error, 1)
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		go func() { done <- cmd.Wait() }()
		var early bool
		select {
		case <-done:
			early = true
		case <-time.After(1500 * time.Millisecond):
		}
		lockRange(t, f, syscall.F_UNLCK, rec, rec)
		if early {
			t.Errorf("dfeedinfo -P finished while the peer's record was locked: the snapshot takes no lock a writer takes")
		} else if err := <-done; err != nil {
			t.Fatalf("dfeedinfo -P: %v", err)
		}
		st, err := os.Stat(path + ".test")
		if err != nil || st.Size() != size {
			t.Errorf("snapshot file: %v, size want %d", err, size)
		}
	})
}

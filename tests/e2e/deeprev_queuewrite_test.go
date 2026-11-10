package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestDeepRevQueueWriteFailureKeepsLines (L1-04): the diablo master collects the queue lines
// of an outgoing feed in a buffer and writes it to dqueue/<label> on a flush. When that
// write failed (full disk, quota, I/O error) flushFeedOut() logged it once and threw the
// buffer away: the articles were accepted and in the history, but never offered to that feed,
// and nothing said which. The lines must stay in the buffer and be written by a later flush.
//
// The queue file is a symbolic link to /dev/full here, where every write fails with ENOSPC;
// after the first failed flush the link is removed, as when space is free again.
func TestDeepRevQueueWriteFailureKeepsLines(t *testing.T) {
	if _, err := os.Stat("/dev/full"); err != nil {
		t.Skip("no /dev/full on this system")
	}
	var qfile string
	s := NewSite(t, SiteOptions{Newsfeeds: OutNewsfeeds, Setup: func(tb testing.TB, s *Site) {
		qfile = filepath.Join(s.Dir, "dqueue", OutLabel)
		if err := os.Symlink("/dev/full", qfile); err != nil {
			tb.Fatal(err)
		}
	}})
	ids := s.Inject(t, "e2e.test", 3)

	// Flush until the master has tried to write the lines and failed.
	limit := time.Now().Add(10 * time.Second)
	for !strings.Contains(generalLog(s), "failure writing to feed "+OutLabel) {
		if time.Now().After(limit) {
			t.Fatalf("diablo did not log a failed write to dqueue/%s (-> /dev/full)", OutLabel)
		}
		s.Dicmd(t, "flush")
		time.Sleep(100 * time.Millisecond)
	}

	if err := os.Remove(qfile); err != nil {
		t.Fatal(err)
	}
	q := deepRevWaitQueue(t, s, OutLabel, len(ids), 5*time.Second)
	if missing := deepRevMissing(q, ids); len(missing) != 0 {
		t.Errorf("after a failed write to dqueue/%s, %d of %d accepted articles never reached the queue: %q\ndqueue/%s: %q",
			OutLabel, len(missing), len(ids), missing, OutLabel, q)
	}
}

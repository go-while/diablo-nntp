package e2e

import (
	"fmt"
	"os"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/go-while/nntp-testkit/client"
)

// deepRevStreamBehindMaster stops the diablo master (SIGSTOP), sends the articles with
// TAKETHIS in one write on a new streaming session, reads the 239 of each and lets the master
// continue. The session process does not need the master to accept an article, and its SOUT
// lines (one per article) wait in the pipe to the master, so the master finds all of them
// there at once, as it does when it is behind under load. The connection is returned open.
func deepRevStreamBehindMaster(t *testing.T, s *Site, ids []string, arts [][]string) *client.Conn {
	t.Helper()
	if runtime.GOOS == "illumos" || runtime.GOOS == "solaris" {
		t.Skip("needs a pipe that holds several KB while the reader is stopped")
	}
	p := s.Peer(t)
	p.ExpectCmd(203, "MODE STREAM")

	if err := s.Proc.Signal(syscall.SIGSTOP); err != nil {
		t.Fatalf("SIGSTOP diablo: %v", err)
	}
	// Also on a failure below: a stopped master cannot be shut down.
	t.Cleanup(func() { _ = s.Proc.Signal(syscall.SIGCONT) })

	var b strings.Builder
	for i, id := range ids {
		b.WriteString("TAKETHIS " + id + "\r\n")
		b.WriteString(client.Stuff(arts[i]))
	}
	p.MustSend(b.String())
	for _, id := range ids {
		p.ExpectStatus(239, "TAKETHIS "+id)
	}
	// The 239 is written after the SOUT line; a moment for the last one all the same.
	time.Sleep(300 * time.Millisecond)
	if err := s.Proc.Signal(syscall.SIGCONT); err != nil {
		t.Fatalf("SIGCONT diablo: %v", err)
	}
	return p
}

// TestDeepRevSoutLinesBehindLongLine (L8-01): the master reads the SOUT lines of a session
// with egets(), which moves a line longer than its one-page buffer into a larger buffer, and
// DoPipe() gives that buffer back after two lines with bextfree() (lib/buffer.c). bextfree()
// copied the unread rest from the start of the large buffer instead of from the read
// position: the SOUT lines that were already read behind the long line were overwritten with
// the start of the long line and never queued for any feed, with no log line.
//
// One article with a SOUT line over 4 KB (as in TestSoutOverlongLine) and three short ones
// behind it: all four must be in the outgoing queue after the session has ended.
func TestDeepRevSoutLinesBehindLongLine(t *testing.T) {
	if os.Getpagesize() != 4096 {
		t.Skipf("page size %d: the long SOUT line of this test is sized for a 4096-byte buffer", os.Getpagesize())
	}
	s := NewSite(t, SiteOptions{Newsfeeds: OutNewsfeeds})
	deepRevKillChildren(t, s, "diablo")

	var groups []string
	for i := 0; i < 150; i++ {
		groups = append(groups, fmt.Sprintf("e2e.overlong.g%03d", i))
	}
	ids := []string{msgID(t), msgID(t), msgID(t), msgID(t)}
	long := newArticle(ids[0], strings.Join(groups, ","), "long SOUT line", "body")
	long[0] = "Path: e2e.peer!" + strings.Repeat("hop.example.invalid!", 90) + "not-for-mail"
	arts := [][]string{long}
	for _, id := range ids[1:] {
		arts = append(arts, newArticle(id, "e2e.test", "short SOUT line behind the long one", "body"))
	}

	p := deepRevStreamBehindMaster(t, s, ids, arts)
	p.ExpectCmd(205, "QUIT")

	q := deepRevWaitQueue(t, s, OutLabel, len(ids), 10*time.Second)
	if missing := deepRevMissing(q, ids); len(missing) > 0 {
		t.Errorf("%d of %d accepted articles were never queued for the feed (all were answered 239): missing %q\n"+
			"article 1 has the long SOUT line, 2 to 4 came behind it in the master's buffer.\ndqueue/%s: %q",
			len(missing), len(ids), missing, OutLabel, q)
	}
}

// TestDeepRevSoutLinesLeftInBuffer (L1-03): DoPipe() handles two SOUT lines per call and
// returns; the master's main loop calls it again only when select() reports the pipe
// readable. Complete lines that the first read() already took into the buffer are not seen
// by select(), so they waited there until the same session accepted another article or
// ended: accepted articles were not queued (and not logged) for as long as the peer stayed
// connected and quiet.
//
// Six short articles in one go, the connection stays open and idle: all six must be in the
// outgoing queue within 10 seconds.
func TestDeepRevSoutLinesLeftInBuffer(t *testing.T) {
	s := NewSite(t, SiteOptions{Newsfeeds: OutNewsfeeds})
	deepRevKillChildren(t, s, "diablo")

	var ids []string
	var arts [][]string
	for i := 0; i < 6; i++ {
		id := msgID(t)
		ids = append(ids, id)
		arts = append(arts, newArticle(id, "e2e.test", fmt.Sprintf("burst %d", i+1), "body"))
	}
	p := deepRevStreamBehindMaster(t, s, ids, arts)

	q := deepRevWaitQueue(t, s, OutLabel, len(ids), 10*time.Second)
	if missing := deepRevMissing(q, ids); len(missing) > 0 {
		t.Errorf("%d of %d accepted articles are not queued 10 s after they were accepted, while their session is idle: missing %q\n"+
			"dqueue/%s: %q", len(missing), len(ids), missing, OutLabel, q)
	}
	p.ExpectCmd(205, "QUIT")
}

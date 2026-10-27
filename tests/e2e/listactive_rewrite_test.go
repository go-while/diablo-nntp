package e2e

import (
	"strings"
	"testing"
	"time"
)

// TestReaderListActiveAfterRecordRewrite (D-54): a reader process that scanned the active file
// before another process rewrote a record (the new record is appended at the end of the file)
// must list the group in its very next LIST ACTIVE. Fails on main: the scan continued one record
// length too far after it noticed the file had grown, so the appended record was skipped and
// the group missing from that one listing.
func TestReaderListActiveAfterRecordRewrite(t *testing.T) {
	st, spool := spoolStore(t)
	s := NewReaderSite(t, ReaderOptions{
		Spool: spool,
		Files: map[string]string{"dcontrol.ctl": "newgroup:*:*:doit\n"},
	})
	c := s.Reader(t)
	defer expect(t, c, 205, "QUIT")
	expect(t, c, 215, "LIST ACTIVE")
	if l := strings.Join(c.ReadDotLines(), "\n"); !strings.Contains(l, "e2e.test") {
		t.Fatalf("warm-up listing lacks e2e.test:\n%s", l)
	}

	before := s.Run(t, "dkp", "-r", s.Dir+"/dactive.kp", "e2e.test")

	// A newgroup control message for a group that exists, with a longer description: the
	// record cannot be rewritten in place and is appended to the file.
	a := newArticle(msgID(t), "e2e.test", "cmsg newgroup e2e.test", "For your newsgroups file:",
		"e2e.test\tA much longer description of the test group so that the record has to grow in the file.")
	a = append(a[:3:3], append([]string{"Control: newgroup e2e.test"}, a[3:]...)...)
	feedReader(t, s, st, "e2e.test", a)
	// wait until the control message has been applied (the file changed)
	applied := false
	for deadline := time.Now().Add(10 * time.Second); time.Now().Before(deadline); time.Sleep(200 * time.Millisecond) {
		if s.Run(t, "dkp", "-r", s.Dir+"/dactive.kp", "e2e.test") != before {
			applied = true
			break
		}
	}
	if !applied {
		t.Fatal("the newgroup control message was not applied within 10 s")
	}

	expect(t, c, 215, "LIST ACTIVE")
	if l := strings.Join(c.ReadDotLines(), "\n"); !strings.Contains(l, "e2e.test") || !strings.Contains(l, "e2e.other") {
		t.Errorf("the first LIST ACTIVE after the rewrite is missing a group:\n%s", l)
	}
}

package e2e

import (
	"strings"
	"testing"
	"time"
)

// TestReaderCheckgroupsControl (D-53): an accepted checkgroups control message updates the
// description of the groups that exist, and does not create the groups it names that do not.
// Fails on main: the unknown group appeared in LIST ACTIVE as `0 0 n` and GROUP answered
// "211 1 -1 -1".
func TestReaderCheckgroupsControl(t *testing.T) {
	st, spool := spoolStore(t)
	s := NewReaderSite(t, ReaderOptions{
		Spool: spool,
		Files: map[string]string{"dcontrol.ctl": "checkgroups:*:*:doit\n"},
	})
	a := newArticle(msgID(t), "e2e.test", "cmsg checkgroups",
		"e2e.ghost\tA group that does not exist.", "e2e.test\tRenamed test group.")
	a = append(a[:3:3], append([]string{"Control: checkgroups"}, a[3:]...)...)
	feedReader(t, s, st, "e2e.test", a)

	c := s.Reader(t)
	defer expect(t, c, 205, "QUIT")
	// the control message is applied by dreaderd after it took the article
	var desc []string
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		expect(t, c, 215, "LIST NEWSGROUPS")
		desc = c.ReadDotLines()
		if strings.Contains(strings.Join(desc, "\n"), "Renamed test group.") {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	all := strings.Join(desc, "\n")
	if !strings.Contains(all, "e2e.test\tRenamed test group.") {
		t.Errorf("the description of the existing group was not updated:\n%s", all)
	}
	if strings.Contains(all, "e2e.ghost") {
		t.Errorf("checkgroups created the group e2e.ghost:\n%s", all)
	}
	expect(t, c, 215, "LIST ACTIVE")
	if act := strings.Join(c.ReadDotLines(), "\n"); strings.Contains(act, "e2e.ghost") {
		t.Errorf("e2e.ghost is in LIST ACTIVE:\n%s", act)
	}
	if code, line := c.Cmd("GROUP e2e.ghost"); code != 411 {
		t.Errorf("GROUP e2e.ghost: %d %q, want 411", code, line)
	}
}

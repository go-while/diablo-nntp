package e2e

import (
	"path/filepath"
	"strconv"
	"testing"
)

// TestDeepRevDsyncgroupsGroupName (L7-06): dsyncgroups -g took every name the remote sent in
// LIST ACTIVE (and LIST NEWSGROUPS) as a newsgroup and created it in dactive.kp, although
// diablo and dreaderd accept no such name anywhere else: a "/" in it becomes part of an
// overview path with "readergrouphash hierarchy", a control character part of every LIST
// answer. Names that ValidGroupName() refuses are now skipped.
func TestDeepRevDsyncgroupsGroupName(t *testing.T) {
	s := NewSite(t, SiteOptions{NoStart: true})
	db := filepath.Join(s.Dir, "deeprev.kp")
	bad := []string{"../../deeprev/up", "deeprev..dots", ".deeprev", "deeprev\x01ctl"}
	var active, descr []string
	for _, g := range bad {
		active = append(active, g+" 10 1 y")
		descr = append(descr, g+"\ta bad name")
	}
	active = append(active, "alpha 10 1 y")
	descr = append(descr, "alpha\tthe good one")
	port := dsyncPeer{Active: active, Newsgroups: descr}.listen(t)
	out := s.Run(t, "dsyncgroups", "-h", "127.0.0.1", "-p", strconv.Itoa(port),
		"-F", db, "-g", "-a", "-i")
	if live, _, _ := kpRecords(t, db, "alpha"); len(live) != 1 {
		t.Errorf("the good group alpha was not created: %q\n%s", live, out)
	}
	for _, g := range bad {
		if live, _, _ := kpRecords(t, db, g); len(live) != 0 {
			t.Errorf("group %q from the remote was created: %q", g, live)
		}
	}
}

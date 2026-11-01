package e2e

import (
	"strings"
	"testing"
)

// offerGroups offers the site an article with the given Newsgroups: header value and returns the
// first status line of the final IHAVE reply.
func offerGroups(t *testing.T, s *Site, newsgroups string) string {
	t.Helper()
	id := msgID(t)
	p := s.Peer(t)
	defer expect(t, p, 205, "QUIT")
	expect(t, p, 335, "IHAVE %s", id)
	send(t, p, wire(newArticle(id, newsgroups, "groups "+newsgroups, "body")))
	line, err := p.ReadLine()
	if err != nil {
		t.Fatalf("IHAVE with Newsgroups: %q: %v", newsgroups, err)
	}
	return line
}

// TestNewsgroupsSpaceAfterComma: a Newsgroups: header may have a space after each comma (RFC
// 5536 allows folding white space there, and many posters write "a, b"). Before the fix every
// group after the first was read as an empty name by the incoming filter, so an article that
// the IFILTER label must not match (delgroupany names one of its groups) was rejected as
// IncomingFilter when that group was not the first.
func TestNewsgroupsSpaceAfterComma(t *testing.T) {
	s := NewSite(t, SiteOptions{
		Newsfeeds: DefaultNewsfeeds + "label IFILTER\n    addgroup *\n    delgroupany e2e.keep\nend\n",
	})
	for _, c := range []struct {
		groups string
		want   string
	}{
		{"e2e.drop", "437"},                       // matches IFILTER: rejected
		{"e2e.drop,e2e.keep", "235"},              // delgroupany e2e.keep: not filtered
		{"e2e.drop, e2e.keep", "235"},             // the same with a space after the comma
		{"e2e.drop,  e2e.other, e2e.keep", "235"}, // two spaces, the group in third place
		{"e2e.drop, e2e.other", "437"},            // no group excluded: still filtered
		{"e2e.keep, e2e.drop", "235"},             // the excluded group first
	} {
		line := offerGroups(t, s, c.groups)
		if !strings.HasPrefix(line, c.want) {
			t.Errorf("IHAVE with Newsgroups: %q: %q, want %s", c.groups, line, c.want)
		}
	}
}

// TestNewsgroupsSpaceActive: with "activedrop yes" an article is kept when any of its groups is
// in the active file, and a moderated group among them makes an unapproved article refused with
// Unapproved/<group>. Both looked only at the groups before the first ", " (a space after a
// comma ended the scan).
func TestNewsgroupsSpaceActive(t *testing.T) {
	s := NewSite(t, SiteOptions{
		Config: map[string]string{
			"active":     "on",
			"activedrop": "yes",
			"xrefhost":   PathHost,
		},
		Setup: func(tb testing.TB, s *Site) {
			s.WriteFile(tb, "dactive.kp", "")
			s.Run(tb, "dgrpctl", "-s", "newgroup", ModGroup, "m")
			s.Run(tb, "dgrpctl", "-s", "newgroup", OpenGroup, "y")
		},
	})
	for _, c := range []struct {
		groups string
		want   string
	}{
		{OpenGroup, "235"},
		{"e2e.nogroup," + OpenGroup, "235"},
		{"e2e.nogroup, " + OpenGroup, "235"}, // the group in the active file after ", "
		{OpenGroup + "," + ModGroup, "437"},  // a moderated group, unapproved
		{OpenGroup + ", " + ModGroup, "437"}, // the same after ", "
	} {
		line := offerGroups(t, s, c.groups)
		if !strings.HasPrefix(line, c.want) {
			t.Errorf("IHAVE with Newsgroups: %q: %q, want %s", c.groups, line, c.want)
		}
	}
}

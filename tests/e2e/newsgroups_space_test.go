package e2e

import (
	"github.com/go-while/nntp-testkit/client"
	"strings"
	"testing"
)

// offerGroups offers the site an article with the given Newsgroups: header value and returns the
// first status line of the final IHAVE reply.
func offerGroups(t *testing.T, s *Site, newsgroups string) string {
	t.Helper()
	id := msgID(t)
	p := s.Peer(t)
	defer p.ExpectCmd(205, "QUIT")
	p.ExpectCmd(335, "IHAVE %s", id)
	p.MustSend(client.Stuff(newArticle(id, newsgroups, "groups "+newsgroups, "body")))
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

// TestNewsgroupsEmptyEntry: an empty entry in the Newsgroups: list (a trailing comma, which
// posting tools write, or ",,") is not a group and is ignored, as INN's NGsplit does. Before, it
// was matched as the empty name, which "addgroup *" matches and "delgroup *.bina*" does not, so
// the sample IFILTER rejected a binary article that ends in a comma; and it counted as a group
// for maxcross.
func TestNewsgroupsEmptyEntry(t *testing.T) {
	s := NewSite(t, SiteOptions{
		Newsfeeds: DefaultNewsfeeds + "label IFILTER\n    addgroup *\n    delgroup *.bina*\nend\n",
	})
	for _, c := range []struct {
		groups string
		want   string
	}{
		{"alt.binaries.nzb,alt.binaries.ftd", "235"},
		{"alt.binaries.nzb,alt.binaries.ftd,", "235"},  // trailing comma
		{"alt.binaries.nzb,,alt.binaries.ftd", "235"},  // empty entry in the middle
		{"alt.binaries.nzb, alt.binaries.ftd,", "235"}, // after a space
		{",alt.binaries.nzb", "235"},                   // leading comma
		{"alt.binaries.nzb,", "235"},                   // one group and a comma
		{"e2e.drop,", "437"},                           // not a binary group: still filtered
		{"e2e.drop,,alt.binaries.nzb", "437"},          // a filtered group among empty entries
	} {
		line := offerGroups(t, s, c.groups)
		if !strings.HasPrefix(line, c.want) {
			t.Errorf("IHAVE with Newsgroups: %q: %q, want %s", c.groups, line, c.want)
		}
	}
}

// TestNewsgroupsEmptyEntryNotCounted: an empty entry does not count as a group for maxcross.
// IFILTER with "maxcross 2" rejects an article of at most two groups.
func TestNewsgroupsEmptyEntryNotCounted(t *testing.T) {
	s := NewSite(t, SiteOptions{
		Newsfeeds: DefaultNewsfeeds + "label IFILTER\n    addgroup *\n    maxcross 2\nend\n",
	})
	for _, c := range []struct {
		groups string
		want   string
	}{
		{"e2e.a,e2e.b", "437"},       // two groups: matched, rejected
		{"e2e.a,e2e.b,", "437"},      // the trailing comma is not a third group
		{"e2e.a,e2e.b,e2e.c", "235"}, // three groups: over maxcross, not matched
	} {
		line := offerGroups(t, s, c.groups)
		if !strings.HasPrefix(line, c.want) {
			t.Errorf("IHAVE with Newsgroups: %q: %q, want %s", c.groups, line, c.want)
		}
	}
}

// TestNewsgroupsEmptyEntryActive: an empty entry in the Newsgroups: list must not stop the active
// and moderated-group scan either. InActive() and FirstModeratedGroup() walk the list themselves
// and broke on an empty entry, so a group listed after one ("a,,b") was never looked up: an
// article kept only by that later group was dropped, and a moderated group hidden behind an empty
// entry let an unapproved article through.
func TestNewsgroupsEmptyEntryActive(t *testing.T) {
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
		{"e2e.nogroup,," + OpenGroup, "235"}, // the active group after an empty entry: still kept
		{",," + OpenGroup, "235"},            // leading empty entries, then the active group
		{OpenGroup + ",," + ModGroup, "437"}, // a moderated group after an empty entry: unapproved
	} {
		line := offerGroups(t, s, c.groups)
		if !strings.HasPrefix(line, c.want) {
			t.Errorf("IHAVE with Newsgroups: %q: %q, want %s", c.groups, line, c.want)
		}
	}
}

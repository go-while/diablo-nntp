package e2e

import (
	"strings"
	"testing"

	"github.com/go-while/nntp-testkit/client"
	"github.com/go-while/nntp-testkit/fakeserver"
)

// TestReaderPostReservedGroups covers RFC 5536 3.1.4 / RFC 5537 3.5 step 4,
// including whole components, crossposts, purpose exceptions and local legacy mode.
func TestReaderPostReservedGroups(t *testing.T) {
	groups := []string{"e2e.test", "example", "example.test", "EXAMPLE.test", "poster", "poster.test",
		"to", "to.peer", "control", "control.cancel", "junk", "all", "e2e.all.test", "ctl", "e2e.ctl",
		"e2e.alligator", "e2e.control", "examplex.test", "examples.foo", "junkyard", "e2e.to", "ctlx"}
	for _, mode := range []struct {
		name, vs string
		strict   bool
	}{
		{"default", "", true},
		{"rfc5537 no", "    rfc5537 no\n", false},
		{"legacytrace yes", "    legacytrace yes\n", false},
	} {
		t.Run(mode.name, func(t *testing.T) {
			_, spool := spoolStore(t)
			peer := fakeserver.New(t, fakeserver.Options{Handler: fakeserver.PeerHandler(&fakeserver.PeerOptions{})})
			access := strings.Replace(plainAccess(mode.vs), "    post yes\n", "    post yes\n    controlpost yes\n", 1)
			s := NewReaderSite(t, ReaderOptions{Spool: spool, Post: peer, Access: access, Groups: groups})
			c := s.Reader(t)
			defer c.Close()
			for _, tc := range []struct {
				group, control string
				reserved       bool
			}{
				{"e2e.test", "", false}, {"example", "", true}, {"example.test", "", true},
				{"EXAMPLE.test", "", true}, {"poster", "", true}, {"poster.test", "", false},
				{"to", "", true}, {"to.peer", "", true}, {"control", "", true},
				{"control.cancel", "", true}, {"junk", "", true}, {"all", "", true},
				{"e2e.all.test", "", true}, {"ctl", "", true}, {"e2e.ctl", "", true},
				{"e2e.alligator", "", false}, {"e2e.control", "", false}, {"examplex.test", "", false},
				{"examples.foo", "", false}, {"junkyard", "", false}, {"e2e.to", "", false}, {"ctlx", "", false},
				{"e2e.test , example.test", "", true}, {"e2e.test, junk", "", true}, {"e2e.test,\tjunk", "", true}, {"e2e.test,control.cancel", "", true},
				{"control.cancel", "newgroup e2e.future", false},
				{"to.peer", "ihave <target@e2e.test>", false}, {"to", "ihave <target@e2e.test>", true},
				{"to.peer", "sendme <target@e2e.test>", false},
				{"to.peer", "newgroup e2e.future", true},
				{"to.peer", "ihavex <target@e2e.test>", true},
				{"example.test", "newgroup e2e.future", true},
			} {
				c.ExpectCmd(340, "POST")
				art := []string{"From: Poster <poster@example.invalid>", "Newsgroups: " + tc.group,
					"Subject: reserved groups", "Followup-To: poster", "Message-ID: " + msgID(t)}
				if tc.control != "" {
					art = append(art, "Control: "+tc.control)
				}
				c.MustSend(client.Stuff(append(art, "", "body")))
				want := 240
				if mode.strict && tc.reserved {
					want = 441
				}
				code, line, err := c.ReadStatus()
				if err != nil {
					t.Fatalf("POST to %q: %v", tc.group, err)
				}
				if code != want {
					t.Errorf("POST to %q with Control %q: %q, want %d", tc.group, tc.control, line, want)
				}
				if code == 441 && !strings.Contains(line, "Reserved newsgroup") {
					t.Errorf("wrong refusal: %q", line)
				}
				c.ExpectCmd(111, "DATE")
			}
		})
	}
}

func TestReaderPostReservedControlPermission(t *testing.T) {
	_, spool := spoolStore(t)
	peer := fakeserver.New(t, fakeserver.Options{Handler: fakeserver.PeerHandler(&fakeserver.PeerOptions{})})
	access := strings.Replace(DefaultReaderAccess, "    post yes\n", "    post yes\n    controlpost no\n", 1)
	s := NewReaderSite(t, ReaderOptions{Spool: spool, Post: peer, Access: access, Groups: []string{"control.test"}})
	c := s.Reader(t)
	defer c.Close()
	c.ExpectCmd(340, "POST")
	c.MustSend(client.Stuff([]string{"From: Poster <poster@example.invalid>", "Newsgroups: control.test",
		"Subject: not authorized", "Message-ID: " + msgID(t), "Control: newgroup e2e.future", "", "body"}))
	code, line, err := c.ReadStatus()
	if err != nil || code != 441 || !strings.Contains(line, "Illegal \"Control\"") {
		t.Errorf("unauthorized control-purpose POST: %q (%v), want 441 Illegal Control", line, err)
	}
}

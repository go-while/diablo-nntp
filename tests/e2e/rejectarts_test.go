package e2e

// rejectartswithbarecr and rejectartswithnul: articles with a CR that is not part of a CRLF,
// or with a NUL byte, are refused when the option is on, and stored as received when it is
// off (the default).

import (
	"strings"
	"testing"

	"github.com/go-while/nntp-testkit/client"
)

// offerBoth offers the body by IHAVE and by TAKETHIS and returns the two final codes.
func offerBoth(t *testing.T, s *Site, body []string) (ihave, takethis int) {
	t.Helper()
	p := s.Peer(t)
	id := msgID(t)
	p.ExpectCmd(335, "IHAVE %s", id)
	p.MustSend(client.Stuff(newArticle(id, "e2e.test", "reject option", body...)))
	code, line, err := p.ReadStatus()
	if err != nil {
		t.Fatalf("IHAVE %s: %v (%q)", id, err, line)
	}
	ihave = code
	p.ExpectCmd(203, "MODE STREAM")
	id = msgID(t)
	p.MustSend("TAKETHIS " + id + "\r\n" + client.Stuff(newArticle(id, "e2e.test", "reject option", body...)))
	code, line, err = p.ReadStatus()
	if err != nil {
		t.Fatalf("TAKETHIS %s: %v (%q)", id, err, line)
	}
	takethis = code
	p.ExpectCmd(205, "QUIT")
	return ihave, takethis
}

// TestRejectArtsWithBareCR: with rejectartswithbarecr on (or 1), an article with a bare CR in
// a body line is refused (IHAVE 437, TAKETHIS 439); off, it is accepted. Before, the option
// was read as a number, so the documented "on" meant off and nothing was refused.
func TestRejectArtsWithBareCR(t *testing.T) {
	testRejectOption(t, "rejectartswithbarecr", []string{"col1\rcol2 on one line", "next line"})
}

// TestRejectArtsWithNul: the same for rejectartswithnul and a NUL byte in the body.
func TestRejectArtsWithNul(t *testing.T) {
	testRejectOption(t, "rejectartswithnul", []string{"before\x00after", "next line"})
}

func testRejectOption(t *testing.T, option string, body []string) {
	for _, c := range []struct {
		value         string
		ihave, stream int
	}{
		{"off", 235, 239},
		{"on", 437, 439},
		{"1", 437, 439},
	} {
		t.Run(c.value, func(t *testing.T) {
			s := NewSite(t, SiteOptions{Config: map[string]string{option: c.value}})
			if out := s.Dicmd(t, "config", option); !strings.Contains(out, option+": "+map[bool]string{true: "1", false: "0"}[c.ihave != 235]) {
				t.Errorf("dicmd config %s: %q", option, out)
			}
			ih, tk := offerBoth(t, s, body)
			if ih != c.ihave || tk != c.stream {
				t.Errorf("%s %s: IHAVE %d, TAKETHIS %d; want %d, %d", option, c.value, ih, tk, c.ihave, c.stream)
			}
		})
	}
}

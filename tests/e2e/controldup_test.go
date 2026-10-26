package e2e

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

// TestControlDuplicateHeadersRefused: dreaderd never executes a control message that has
// Control:, From:, Newsgroups: or X-PGP-Sig: twice, and logs why. It acts on the last
// Control: and matches the last From:/Newsgroups: against dcontrol.ctl, while a signature
// covers the first, so such a message could carry an unsigned command through a good
// signature (GHSA-3g54-68r8-rx6c). A control message with each header once is executed.
func TestControlDuplicateHeadersRefused(t *testing.T) {
	st, spool := spoolStore(t)
	s := NewReaderSite(t, ReaderOptions{
		Spool:  spool,
		Config: map[string]string{"path_generallog": "%s/general.log"},
		Files:  map[string]string{"dcontrol.ctl": "newgroup:*:*:doit\n"},
	})
	control := func(groups ...string) []string {
		a := newArticle(msgID(t), "e2e.test", "cmsg newgroup "+groups[0],
			"For your newsgroups file:", groups[0]+"\tA test group.")
		var head []string
		for _, l := range a {
			if l == "" {
				break
			}
			head = append(head, l)
		}
		for _, g := range groups {
			head = append(head, "Control: newgroup "+g)
		}
		return append(append(head, ""), a[len(head)+1:]...)
	}
	exists := func(g string) bool {
		c := s.Reader(t)
		defer expect(t, c, 205, "QUIT")
		expect(t, c, 215, "LIST ACTIVE %s", g)
		return slices.ContainsFunc(c.ReadDotLines(), func(l string) bool { return strings.HasPrefix(l, g+" ") })
	}

	feedReader(t, s, st, "e2e.test", control("e2e.single"))
	waitFor(t, 10*time.Second, "e2e.single created by a normal control message", func() bool { return exists("e2e.single") })

	feedReader(t, s, st, "e2e.test", control("e2e.first", "e2e.second"))
	time.Sleep(time.Second)
	for _, g := range []string{"e2e.first", "e2e.second"} {
		if exists(g) {
			t.Errorf("%s created by a control message with two Control: headers", g)
		}
	}
	b, _ := os.ReadFile(filepath.Join(s.Dir, "log", "general.log"))
	if !strings.Contains(string(b), "refused: duplicate Control:") {
		t.Errorf("no log line about the refused control message")
	}
}

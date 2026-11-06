package e2e

import (
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-while/nntp-testkit/client"
	"github.com/go-while/nntp-testkit/fakeserver"
)

// groupsAccess is a reader whose readerdef "groups" allows e2e.test only.
const groupsAccess = `vserverdef DEFAULT
    hostname ` + PathHost + `
end
groupdef onlytest
    group e2e.test
end
readerdef rp
    read yes
    post yes
    groups onlytest
end
readerdef feed
    feed yes
end
access 127.0.0.1 rp
access ` + PeerIP + ` feed
`

// TestReaderGroupsByMessageId (D-195): a readerdef "groups" list limits what a reader may
// fetch by message-id, as INN's PERMartok() does. ARTICLE, HEAD, BODY, STAT and HDR/XHDR
// <message-id> of an article in no allowed group answer 502: from the spool, from the reader
// cache, for an article only the spool has (folded Newsgroups, Xref), and read from a
// localspool file. An article crossposted to an allowed group, one without Xref and
// Newsgroups, and an unknown id (430) are answered as before. The connection stays
// usable after each refusal, and a reader without a groups list is not limited. Before,
// "groups" only applied to GROUP and LISTGROUP.
func TestReaderGroupsByMessageId(t *testing.T) {
	for _, cache := range []string{"off", "on"} {
		t.Run("readercache "+cache, func(t *testing.T) {
			st, spool := spoolStore(t)
			st.AddGroup("e2e.other", "e2e other group")
			s := NewReaderSite(t, ReaderOptions{Spool: spool, Access: groupsAccess,
				Config: map[string]string{"readercache": cache}})
			allowed, other, cross, spoolOnly := msgID(t), msgID(t), msgID(t), msgID(t)
			feedReader(t, s, st, "e2e.test", newArticle(allowed, "e2e.test", "allowed subject", "allowed body"))
			feedReader(t, s, st, "e2e.other", newArticle(other, "e2e.other", "secret subject", "secret body"))
			feedReader(t, s, st, "e2e.other", newArticle(cross, "e2e.other,e2e.test", "cross subject", "cross body"))
			// only in the spool, with a folded Newsgroups and an Xref naming e2e.other
			st.AddArticle("e2e.other", fakeserver.Article{MessageID: spoolOnly, Headers: []string{
				"Path: e2e.peer!not-for-mail", "From: E2E <e2e@example.invalid>",
				"Newsgroups: e2e.other,", "\te2e.more", "Subject: spool secret",
				"Message-ID: " + spoolOnly, "Xref: e2e.peer e2e.other:7 e2e.more:3",
			}, Body: []string{"spool secret body"}})

			c := s.Reader(t)
			defer c.ExpectCmd(205, "QUIT")
			c.ExpectCmd(411, "GROUP e2e.other")
			// twice: the second time from the reader cache when it is on
			for round := 0; round < 2; round++ {
				for _, id := range []string{other, spoolOnly} {
					for _, cmd := range []string{"ARTICLE", "HEAD", "BODY", "STAT"} {
						if code, line := c.Cmd("%s %s", cmd, id); code != 502 {
							t.Errorf("round %d: %s %s: %q, want 502", round, cmd, id, line)
							if code >= 220 && code <= 222 {
								c.ReadDotLines()
							}
						}
					}
					for _, cmd := range []string{"HDR Subject", "XHDR Subject"} {
						if code, line := c.Cmd("%s %s", cmd, id); code != 502 {
							t.Errorf("round %d: %s %s: %q, want 502", round, cmd, id, line)
							if code == 221 || code == 225 {
								c.ReadDotLines()
							}
						}
					}
				}
				// readable: the allowed group and a crosspost to it
				for _, tc := range []struct{ id, body string }{
					{allowed, "allowed body"}, {cross, "cross body"},
				} {
					c.ExpectCmd(222, "BODY %s", tc.id)
					if got := c.ReadDotLines(); len(got) != 1 || got[0] != tc.body {
						t.Errorf("round %d: BODY %s: %q, want [%q]", round, tc.id, got, tc.body)
					}
					c.ExpectCmd(223, "STAT %s", tc.id)
				}
				c.ExpectCmd(430, "ARTICLE <unknown-d195@e2e.test>")
			}
			c.ExpectCmd(211, "GROUP e2e.test")
			c.ExpectCmd(225, "HDR Subject %s", cross)
			if got := c.ReadDotLines(); len(got) != 1 || got[0] != "0 cross subject" {
				t.Errorf("HDR Subject %s in e2e.test: %q", cross, got)
			}
		})
	}

	t.Run("no groups list", func(t *testing.T) {
		st, spool := spoolStore(t)
		s := NewReaderSite(t, ReaderOptions{Spool: spool})
		id := msgID(t)
		feedReader(t, s, st, "e2e.other", newArticle(id, "e2e.other", "open subject", "open body"))
		c := s.Reader(t)
		defer c.ExpectCmd(205, "QUIT")
		c.ExpectCmd(220, "ARTICLE %s", id)
		c.ReadDotLines()
		c.ExpectCmd(225, "HDR Subject %s", id)
		c.ReadDotLines()
	})

	t.Run("localspool", func(t *testing.T) {
		// dreaderd reads the article from the file the spool's "whereis" names
		dir := t.TempDir()
		l := localSpoolServer(t, dir, 0)
		_, port, _ := net.SplitHostPort(l.Addr().String())
		s := NewReaderSite(t, ReaderOptions{SpoolAddr: l.Addr().String(), NoStart: true, Access: groupsAccess,
			Files: map[string]string{"dserver.hosts": "127.0.0.1\ts1p" + port + " localspool=" + dir + "\n"}})
		startReader(t, s, "[127.0.0.1]:"+strings.Split(s.Addr, ":")[1])
		p := s.dialFrom(t, PeerIP)
		ids := map[string]string{}
		for _, g := range []string{"e2e.test", "e2e.other"} {
			id := msgID(t)
			ids[g] = id
			art := newArticle(id, g, "localspool "+g, "body of "+g)
			if err := os.WriteFile(filepath.Join(dir, strings.Trim(id, "<>")+".art"),
				[]byte(strings.Join(art, "\n")+"\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			p.ExpectCmd(335, "IHAVE %s", id)
			p.MustSend(client.Stuff(art))
			p.ExpectStatus(235, "IHAVE")
		}
		// an article with neither Xref nor Newsgroups is readable, as with INN (the fake
		// spool of the other subtests adds a Newsgroups header, a file here has none)
		noGroups := msgID(t)
		if err := os.WriteFile(filepath.Join(dir, strings.Trim(noGroups, "<>")+".art"),
			[]byte("From: E2E <e2e@example.invalid>\nSubject: no groups\nMessage-ID: "+noGroups+"\n\nno groups body\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		c := s.Reader(t)
		defer c.ExpectCmd(205, "QUIT")
		c.ExpectCmd(222, "BODY %s", noGroups)
		if got := c.ReadDotLines(); len(got) != 1 || got[0] != "no groups body" {
			t.Errorf("localspool BODY of an article without Xref and Newsgroups: %q", got)
		}
		for _, cmd := range []string{"ARTICLE", "HEAD", "BODY", "STAT"} {
			if code, line := c.Cmd("%s %s", cmd, ids["e2e.other"]); code != 502 {
				t.Errorf("localspool %s of an e2e.other article: %q, want 502", cmd, line)
				if code >= 220 && code <= 222 {
					c.ReadDotLines()
				}
			}
		}
		c.ExpectCmd(222, "BODY %s", ids["e2e.test"])
		if got := c.ReadDotLines(); len(got) != 1 || got[0] != "body of e2e.test" {
			t.Errorf("localspool BODY of the e2e.test article: %q", got)
		}
	})
}

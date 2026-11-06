package e2e

import (
	"regexp"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	testkit "github.com/go-while/nntp-testkit"
	"github.com/go-while/nntp-testkit/client"
	"github.com/go-while/nntp-testkit/fakeserver"
)

// injAccess is DefaultReaderAccess with these vserverdef lines added.
func injAccess(vs string) string {
	return strings.Replace(DefaultReaderAccess, "    hostname "+PathHost+"\n",
		"    hostname "+PathHost+"\n"+vs, 1)
}

// postHead posts an article with these header lines through a reader with access and
// returns the header the outgoing server got, with folded lines joined.
func postHead(t *testing.T, access string, hdrs ...string) []string {
	t.Helper()
	_, spool := spoolStore(t)
	var mu sync.Mutex
	var arts [][]string
	post := fakeserver.New(t, fakeserver.Options{Handler: fakeserver.PeerHandler(&fakeserver.PeerOptions{
		OnArticle: func(id string, wire []string) {
			mu.Lock()
			defer mu.Unlock()
			arts = append(arts, wire)
		},
	})})
	s := NewReaderSite(t, ReaderOptions{Spool: spool, Post: post, Access: access})
	c := s.Reader(t)
	c.ExpectCmd(340, "POST")
	c.MustSend(client.Stuff(append(append([]string{
		"From: E2E Poster <poster@example.invalid>",
		"Newsgroups: e2e.test",
		"Subject: injection info",
	}, hdrs...), "", "body")))
	c.ExpectStatus(240, "POST")
	testkit.WaitFor(t, 15*time.Second, "the outgoing server to receive the post", func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(arts) > 0
	})
	mu.Lock()
	head, _ := splitArticle(slices.Clone(arts[0]))
	mu.Unlock()
	var joined []string
	for _, l := range head {
		if (strings.HasPrefix(l, " ") || strings.HasPrefix(l, "\t")) && len(joined) > 0 {
			joined[len(joined)-1] += " " + strings.TrimSpace(l)
			continue
		}
		joined = append(joined, l)
	}
	return joined
}

func hasHeader(head []string, name string) bool {
	for _, l := range head {
		if k, _, ok := strings.Cut(l, ":"); ok && strings.EqualFold(k, name) {
			return true
		}
	}
	return false
}

var injDateRe = regexp.MustCompile(`^(Mon|Tue|Wed|Thu|Fri|Sat|Sun), [0-3][0-9] (Jan|Feb|Mar|Apr|May|Jun|Jul|Aug|Sep|Oct|Nov|Dec) [0-9]{4} [0-2][0-9]:[0-5][0-9]:[0-6][0-9] \+0000$`)

// TestReaderPostInjectionInfo (F-07 step 2, RFC 5537 3.5 steps 9 to 11): a post gets a Path
// with the "!.POSTED" diagnostic, an Injection-Info with posting-host, logging-data and
// mail-complaints-to, and no NNTP-Posting-Host, X-Trace or X-Complaints-To. Injection-Date
// is added only where dreaderd supplied the Message-ID or Date, and a poster's own is kept.
// Before, the old trace headers were written and no Injection-* header.
func TestReaderPostInjectionInfo(t *testing.T) {
	access := injAccess("    postpath " + PathHost + "\n    abuseto abuse@example.invalid\n")

	t.Run("default", func(t *testing.T) {
		head := postHead(t, access, "Message-ID: "+msgID(t), "Date: "+time.Now().UTC().Format(time.RFC1123Z))
		if p := headerValue(head, "Path"); p != PathHost+"!.POSTED!not-for-mail" {
			t.Errorf("Path %q, want %s!.POSTED!not-for-mail", p, PathHost)
		}
		ii := headerValue(head, "Injection-Info")
		for _, want := range []string{PathHost + ";", `posting-host="`, "127.0.0.1", `logging-data="`, `mail-complaints-to="abuse@example.invalid"`} {
			if !strings.Contains(ii, want) {
				t.Errorf("Injection-Info %q, want it to contain %s", ii, want)
			}
		}
		if !strings.HasPrefix(ii, PathHost+";") {
			t.Errorf("Injection-Info %q does not start with the path-identity %s", ii, PathHost)
		}
		// posting-host is a <host-value>: the address, or name:address, without a port
		if m := regexp.MustCompile(`posting-host="([^"]*)"`).FindStringSubmatch(ii); m == nil ||
			!(m[1] == "127.0.0.1" || regexp.MustCompile(`^[A-Za-z0-9.-]+:127\.0\.0\.1$`).MatchString(m[1])) {
			t.Errorf("Injection-Info %q: posting-host is not 127.0.0.1 or name:127.0.0.1", ii)
		}
		for _, h := range []string{"NNTP-Posting-Host", "X-Trace", "X-Complaints-To", "Injection-Date"} {
			if hasHeader(head, h) {
				t.Errorf("post has %s: %q (Message-ID and Date were the poster's)", h, headerValue(head, h))
			}
		}
	})

	t.Run("dreaderd adds the Message-ID", func(t *testing.T) {
		head := postHead(t, access)
		if d := headerValue(head, "Injection-Date"); !injDateRe.MatchString(d) {
			t.Errorf("Injection-Date %q, want an RFC 5322 date in +0000", d)
		}
	})

	t.Run("the poster's Injection-Date is kept", func(t *testing.T) {
		const own = "Mon, 02 Nov 2026 10:00:00 +0000"
		head := postHead(t, access, "Injection-Date: "+own)
		var dates []string
		for _, l := range head {
			if k, v, ok := strings.Cut(l, ":"); ok && strings.EqualFold(k, "Injection-Date") {
				dates = append(dates, strings.TrimSpace(v))
			}
		}
		if len(dates) != 1 || dates[0] != own {
			t.Errorf("Injection-Date %q, want only the poster's %q", dates, own)
		}
	})

	t.Run("cryptpw", func(t *testing.T) {
		for _, pw := range []string{"secret", "G=7,P=secret"} {
			head := postHead(t, injAccess("    postpath "+PathHost+"\n    cryptpw "+pw+"\n"), "Message-ID: "+msgID(t))
			ii := headerValue(head, "Injection-Info")
			want := `logging-data="C=`
			if strings.HasPrefix(pw, "G=") {
				want = `logging-data="G=7,C=`
			}
			if !strings.Contains(ii, `posting-account="`) || strings.Contains(ii, "posting-host=") ||
				!strings.Contains(ii, want) || strings.Contains(ii, "127.0.0.1") {
				t.Errorf("cryptpw %s: Injection-Info %q, want posting-account, no posting-host, %s..., no address", pw, ii, want)
			}
		}
	})

	t.Run("no postpath", func(t *testing.T) {
		head := postHead(t, DefaultReaderAccess, "Message-ID: "+msgID(t))
		if p := headerValue(head, "Path"); p != "not-for-mail" {
			t.Errorf("Path %q without a postpath, want not-for-mail", p)
		}
		if ii := headerValue(head, "Injection-Info"); !strings.HasPrefix(ii, PathHost+";") {
			t.Errorf("Injection-Info %q, want the hostname %s as path-identity", ii, PathHost)
		}
	})

	t.Run("postpath with two elements", func(t *testing.T) {
		// the Path grows to the left: the injecting agent is the element right before
		// ".POSTED", and Injection-Info names that one
		head := postHead(t, injAccess("    postpath outer.example!"+PathHost+"\n"), "Message-ID: "+msgID(t))
		if p := headerValue(head, "Path"); p != "outer.example!"+PathHost+"!.POSTED!not-for-mail" {
			t.Errorf("Path %q, want outer.example!%s!.POSTED!not-for-mail", p, PathHost)
		}
		if ii := headerValue(head, "Injection-Info"); !strings.HasPrefix(ii, PathHost+";") {
			t.Errorf("Injection-Info %q, want the element before .POSTED (%s) as path-identity", ii, PathHost)
		}
	})

	t.Run("legacytrace", func(t *testing.T) {
		head := postHead(t, injAccess("    postpath "+PathHost+"\n    abuseto abuse@example.invalid\n    legacytrace yes\n"))
		if p := headerValue(head, "Path"); p != PathHost+"!not-for-mail" {
			t.Errorf("legacytrace: Path %q, want %s!not-for-mail", p, PathHost)
		}
		for _, h := range []string{"NNTP-Posting-Host", "X-Trace", "X-Complaints-To"} {
			if !hasHeader(head, h) {
				t.Errorf("legacytrace: no %s header", h)
			}
		}
		for _, h := range []string{"Injection-Info", "Injection-Date"} {
			if hasHeader(head, h) {
				t.Errorf("legacytrace: post has %s: %q", h, headerValue(head, h))
			}
		}
	})
}

// TestReaderModeratorGetsNoInjectionInfo (F-07, RFC 5537 3.5 step 7): a post to a moderated
// group is forwarded before Injection-Info and Injection-Date are added, so the moderator's
// copy has neither and the moderator's own post of it is not refused as already injected.
func TestReaderModeratorGetsNoInjectionInfo(t *testing.T) {
	m := newModerationReader(t, ModOptions{Access: injAccess("    postpath " + PathHost + "\n")})
	if m.Mail == nil {
		t.Skip("needs ci/fake-sendmail as the system's sendmail")
	}
	c := m.Reader(t)
	c.ExpectCmd(340, "POST")
	c.MustSend(client.Stuff([]string{
		"From: E2E Poster <poster@example.invalid>",
		"Newsgroups: " + ModGroup,
		"Subject: for the moderator",
		"",
		"body",
	}))
	c.ExpectStatus(240, "POST to a moderated group")
	mails := m.Mail.waitMails(t, 1)
	head, _, _ := strings.Cut(mails[0].Msg, "\n\n")
	for _, h := range []string{"Injection-Info:", "Injection-Date:"} {
		if strings.Contains(strings.ToLower(head), strings.ToLower(h)) {
			t.Errorf("the moderator's copy has %s\n%s", h, head)
		}
	}
	if !strings.Contains(head, "Message-ID:") {
		t.Errorf("the moderator's copy has no Message-ID\n%s", head)
	}
}

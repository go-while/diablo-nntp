package e2e

// Moderated groups: dreaderd mails an unapproved POST to the group's moderator (moderators
// file, sendmail), and diablo with "activedrop" drops an unapproved article to a moderated
// group.
//
// The tests of today's behaviour pass on main. The comments marked "changes with" name the
// assertions that the maintainer's planned moderation PRs change; the tests for the new
// behaviour are written and skipped until those PRs are in (search "PLANNED").
//
// Mail content is checked only where ci/fake-sendmail is installed as the system's sendmail
// (ci/build-on-host with E2E_FAKE_SENDMAIL=1; GitHub Actions does that). Elsewhere those
// assertions are skipped and the rest of each test still runs: the reply codes and what the
// outgoing server receives.

import (
	"bytes"
	"net"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-while/nntp-testkit/fakeserver"
)

// sendmailPaths are the SENDMAIL_PATH values of lib/config.h.
var sendmailPaths = []string{"/usr/sbin/sendmail", "/usr/lib/sendmail"}

// fakeSendmailMarker is the line ci/fake-sendmail starts with after "#!/bin/sh".
const fakeSendmailMarker = "# diablo-e2e-fake-sendmail"

// haveFakeSendmail reports whether the system's sendmail is ci/fake-sendmail.
func haveFakeSendmail() bool {
	for _, p := range sendmailPaths {
		b, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		return bytes.Contains(b, []byte(fakeSendmailMarker))
	}
	return false
}

// mailbox is what ci/fake-sendmail wrote for a site: <site>/mail/N/{argv,msg,done}.
type mailbox struct {
	dir string
}

// mail is one message handed to sendmail.
type mail struct {
	Argv []string // argv[0] first
	Msg  string   // stdin
}

// To returns the To: header of the message.
func (m mail) To() string {
	head, _, _ := strings.Cut(m.Msg, "\n\n")
	for _, l := range strings.Split(head, "\n") {
		if k, v, ok := strings.Cut(l, ":"); ok && strings.EqualFold(k, "To") {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

// newMailbox returns the site's mailbox, or nil where the fake sendmail isn't installed.
func newMailbox(s *Site) *mailbox {
	if !haveFakeSendmail() {
		return nil
	}
	return &mailbox{dir: filepath.Join(s.Dir, "mail")}
}

// setExit makes the fake sendmail exit with code for this site.
func (m *mailbox) setExit(tb testing.TB, code int) {
	tb.Helper()
	if err := os.MkdirAll(m.dir, 0o755); err != nil {
		tb.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(m.dir, "exit"), []byte(strconv.Itoa(code)+"\n"), 0o644); err != nil {
		tb.Fatal(err)
	}
}

// mails returns the complete messages so far, in order.
func (m *mailbox) mails() []mail {
	ents, _ := os.ReadDir(m.dir)
	var nums []int
	for _, e := range ents {
		if n, err := strconv.Atoi(e.Name()); err == nil && e.IsDir() {
			if _, err := os.Stat(filepath.Join(m.dir, e.Name(), "done")); err == nil {
				nums = append(nums, n)
			}
		}
	}
	sort.Ints(nums)
	var out []mail
	for _, n := range nums {
		d := filepath.Join(m.dir, strconv.Itoa(n))
		argv, _ := os.ReadFile(filepath.Join(d, "argv"))
		msg, _ := os.ReadFile(filepath.Join(d, "msg"))
		out = append(out, mail{Argv: strings.Split(strings.TrimSuffix(string(argv), "\n"), "\n"), Msg: string(msg)})
	}
	return out
}

// waitMails waits until there are n messages, then a moment longer for unexpected extra
// ones, and returns them.
func (m *mailbox) waitMails(tb testing.TB, n int) []mail {
	tb.Helper()
	waitFor(tb, 10*time.Second, strconv.Itoa(n)+" mails", func() bool { return len(m.mails()) >= n })
	time.Sleep(500 * time.Millisecond)
	return m.mails()
}

// Groups of the moderation reader site, by status.
const (
	ModGroup    = "e2e.mod"    // m, moderators: e2e-mod@moderators.example.invalid
	ModGroup2   = "e2e.mod2"   // m, moderators: mod2@moderators.example.invalid
	NoModGroup  = "e2e.nomod"  // m, no moderators line
	OpenGroup   = "e2e.test"   // y
	ClosedGroup = "e2e.closed" // n
)

// DefaultModerators maps the moderated groups; "%s" is the group with dots made dashes.
const DefaultModerators = ModGroup + ":%s@moderators.example.invalid\n" +
	ModGroup2 + ":mod2@moderators.example.invalid\n"

// ModOptions configures newModerationReader.
type ModOptions struct {
	Moderators string // the moderators file; "" = DefaultModerators
	Access     string // dreader.access; "" = DefaultReaderAccess
}

// modSite is a reader site with moderated groups, an outgoing ('o') server that records what
// dreaderd passes on, and the site's mailbox (nil without the fake sendmail).
type modSite struct {
	*Site
	Mail *mailbox

	mu     sync.Mutex
	posted [][]string // articles the outgoing server received, as wire lines
}

// Posted returns the articles the outgoing server has received.
func (m *modSite) Posted() [][]string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return slices.Clone(m.posted)
}

// feederLogs returns diablo's incoming log (accept/reject lines with the reason), its
// general log (with path_generallog %s/general.log) and its stderr.
func feederLogs(s *Site) string {
	var b strings.Builder
	for _, f := range []string{"incoming.log", "general.log"} {
		if l, err := os.ReadFile(filepath.Join(s.Dir, "log", f)); err == nil {
			b.Write(l)
		}
	}
	return b.String() + s.Proc.Stderr()
}

// noPost checks that nothing reaches the outgoing server for a while.
func (m *modSite) noPost(tb testing.TB, what string) {
	tb.Helper()
	time.Sleep(2 * time.Second)
	if p := m.Posted(); len(p) > 0 {
		tb.Errorf("%s: the outgoing server received %d article(s), want none", what, len(p))
	}
}

// waitPost waits until the outgoing server has n articles.
func (m *modSite) waitPost(tb testing.TB, n int, what string) [][]string {
	tb.Helper()
	waitFor(tb, 15*time.Second, what, func() bool { return len(m.Posted()) >= n })
	return m.Posted()
}

// newModerationReader starts dreaderd with the groups above, a moderators file and an
// outgoing server.
func newModerationReader(tb testing.TB, o ModOptions) *modSite {
	tb.Helper()
	_, spool := spoolStore(tb)
	m := &modSite{}
	post := fakeserver.New(tb, fakeserver.Options{Handler: fakeserver.PeerHandler(&fakeserver.PeerOptions{
		OnArticle: func(id string, wire []string) {
			m.mu.Lock()
			defer m.mu.Unlock()
			m.posted = append(m.posted, wire)
		},
	})})
	s := NewReaderSite(tb, ReaderOptions{
		Spool:   spool,
		Post:    post,
		Access:  o.Access,
		Groups:  []string{OpenGroup},
		NoStart: true,
	})
	for g, flags := range map[string]string{ModGroup: "m", ModGroup2: "m", NoModGroup: "m", ClosedGroup: "n"} {
		s.Run(tb, "dgrpctl", "newgroup", g, flags)
	}
	s.WriteFile(tb, "moderators", or(o.Moderators, DefaultModerators))
	_, port, _ := net.SplitHostPort(s.Addr)
	startReader(tb, s, "[127.0.0.1]:"+port)
	m.Site = s
	m.Mail = newMailbox(s)
	return m
}

// postArticle POSTs an article to groups (with extra headers) and returns the reply line
// after the article.
func (m *modSite) postArticle(tb testing.TB, groups string, extra ...string) (string, string) {
	tb.Helper()
	c := m.Reader(tb)
	id := msgID(tb)
	expect(tb, c, 340, "POST")
	art := append([]string{
		"From: E2E Poster <poster@example.invalid>",
		"Newsgroups: " + groups,
		"Subject: moderation test",
		"Message-ID: " + id,
		"Path: not-for-mail",
	}, extra...)
	art = append(art, "", "body line one", ".dot line")
	send(tb, c, wire(art))
	line, err := c.ReadLine()
	if err != nil {
		tb.Fatalf("reply to POST: %v", err)
	}
	return id, line
}

// TestModerationMailsModerator: an unapproved POST to a moderated group is mailed to the
// group's moderator ("%s" in the moderators file is the group with dots made dashes) with
// "240 Article mailed to moderator", and nothing reaches the outgoing server. The mail has
// the article without Path:.
func TestModerationMailsModerator(t *testing.T) {
	m := newModerationReader(t, ModOptions{})
	_, line := m.postArticle(t, ModGroup)
	if !strings.HasPrefix(line, "240 ") || !strings.Contains(line, "moderator") {
		t.Fatalf("POST to %s: %q, want 240 ... mailed to moderator", ModGroup, line)
	}
	m.noPost(t, "unapproved post to a moderated group")
	if m.Mail == nil {
		t.Log("no fake sendmail: mail content not checked")
		return
	}
	mails := m.Mail.waitMails(t, 1)
	if len(mails) != 1 {
		t.Fatalf("%d mails, want 1", len(mails))
	}
	got := mails[0]
	if want := "e2e-mod@moderators.example.invalid"; got.To() != want {
		t.Errorf("To: %q, want %q", got.To(), want)
	}
	// changes with the moderator-mail PR: the address as an argument instead of -t
	if !slices.Contains(got.Argv, "-t") {
		t.Errorf("sendmail argv %q, want -t (today's behaviour)", got.Argv)
	}
	if !strings.Contains(got.Msg, "Subject: moderation test") || !strings.Contains(got.Msg, "\nbody line one\n") {
		t.Errorf("mail lacks the article:\n%s", got.Msg)
	}
	if strings.Contains(got.Msg, "\nPath:") {
		t.Errorf("mail has a Path: header:\n%s", got.Msg)
	}
	if !strings.Contains(got.Msg, "\n.dot line\n") {
		t.Errorf("mail has the dot-stuffed line, want it unstuffed:\n%s", got.Msg)
	}
}

// TestModerationApprovedGoesIn: a POST with Approved: goes straight to the outgoing server,
// from any reader. Changes with the "approve" PR: readerdef without "approve yes" gets 441.
func TestModerationApprovedGoesIn(t *testing.T) {
	m := newModerationReader(t, ModOptions{})
	id, line := m.postArticle(t, ModGroup, "Approved: poster@example.invalid")
	if !strings.HasPrefix(line, "240 ") {
		t.Fatalf("approved POST to %s: %q, want 240 (today's behaviour)", ModGroup, line)
	}
	p := m.waitPost(t, 1, "the approved post at the outgoing server")
	if head, _ := splitArticle(p[0]); headerValue(head, "Message-ID") != id {
		t.Errorf("outgoing server got %q, want %s", headerValue(head, "Message-ID"), id)
	}
	if m.Mail != nil && len(m.Mail.mails()) != 0 {
		t.Errorf("approved post was mailed to a moderator")
	}
}

// TestModerationCrosspost: an unapproved crosspost to two moderated groups is mailed to
// both moderators today. Changes with the moderator-mail PR: only the first moderated
// group's moderator (RFC 5537 section 3.5.1).
func TestModerationCrosspost(t *testing.T) {
	m := newModerationReader(t, ModOptions{})
	_, line := m.postArticle(t, ModGroup+","+ModGroup2+","+OpenGroup)
	if !strings.HasPrefix(line, "240 ") {
		t.Fatalf("crosspost: %q, want 240", line)
	}
	m.noPost(t, "unapproved crosspost to moderated groups")
	if m.Mail == nil {
		t.Log("no fake sendmail: mail count not checked")
		return
	}
	mails := m.Mail.waitMails(t, 2)
	var to []string
	for _, ml := range mails {
		to = append(to, ml.To())
	}
	want := []string{"e2e-mod@moderators.example.invalid", "mod2@moderators.example.invalid"}
	if !slices.Equal(to, want) {
		t.Errorf("mailed to %q, want %q (today: every moderated group's moderator)", to, want)
	}
}

// TestModerationNoModerator: an unapproved POST to a moderated group without a moderators
// line is not mailed but passed on to the outgoing server (where diablo with activedrop
// drops it). Changes with the moderator-mail PR: a clear 441.
func TestModerationNoModerator(t *testing.T) {
	m := newModerationReader(t, ModOptions{})
	_, line := m.postArticle(t, NoModGroup)
	if !strings.HasPrefix(line, "240 ") {
		t.Fatalf("POST to %s: %q, want 240 (today's behaviour)", NoModGroup, line)
	}
	m.waitPost(t, 1, "the post passed on to the outgoing server (today's behaviour)")
}

// TestModerationClosedGroup: a POST to a status "n" group is accepted and passed on today.
// Changes with the group-status PR: refused.
func TestModerationClosedGroup(t *testing.T) {
	m := newModerationReader(t, ModOptions{})
	_, line := m.postArticle(t, ClosedGroup)
	if !strings.HasPrefix(line, "240 ") {
		t.Fatalf("POST to %s: %q, want 240 (today's behaviour)", ClosedGroup, line)
	}
	m.waitPost(t, 1, "the post to a closed group at the outgoing server (today's behaviour)")
}

// TestModerationMailFailure: when sendmail fails, dreaderd still answers "240 Article
// mailed to moderator" today. Changes with the moderator-mail PR: the MTA's exit status is
// checked. Needs the fake sendmail.
func TestModerationMailFailure(t *testing.T) {
	m := newModerationReader(t, ModOptions{})
	if m.Mail == nil {
		t.Skip("needs ci/fake-sendmail as the system's sendmail")
	}
	m.Mail.setExit(t, 75) // EX_TEMPFAIL
	_, line := m.postArticle(t, ModGroup)
	if !strings.HasPrefix(line, "240 ") {
		t.Fatalf("POST with a failing sendmail: %q, want 240 (today: the failure is ignored)", line)
	}
	m.Mail.waitMails(t, 1)
}

// TestFeederUnapprovedModerated: diablo with "active on", "activedrop yes" and an xrefhost
// drops an unapproved article to a moderated group; today the reason is "NotInActive",
// although the group is in the active file. Changes with the diablo PR: "Unapproved".
func TestFeederUnapprovedModerated(t *testing.T) {
	s := NewSite(t, SiteOptions{
		Config: map[string]string{
			"active":          "on",
			"activedrop":      "yes",
			"xrefhost":        PathHost,
			"path_generallog": "%s/general.log",
		},
		Setup: func(tb testing.TB, s *Site) {
			s.WriteFile(tb, "dactive.kp", "")
			s.Run(tb, "dgrpctl", "-s", "newgroup", ModGroup, "m")
			s.Run(tb, "dgrpctl", "-s", "newgroup", OpenGroup, "y")
		},
	})
	p := s.Peer(t)
	unapproved := msgID(t)
	expect(t, p, 335, "IHAVE %s", unapproved)
	send(t, p, wire(newArticle(unapproved, ModGroup, "unapproved", "body")))
	line, err := p.ReadLine()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(line, "437") {
		t.Errorf("unapproved IHAVE to %s: %q, want 437", ModGroup, line)
	}
	open := msgID(t)
	expect(t, p, 335, "IHAVE %s", open)
	send(t, p, wire(newArticle(open, OpenGroup, "open", "body")))
	status(t, p, 235, "IHAVE to an unmoderated group")
	expect(t, p, 205, "QUIT")

	// changes with the diablo PR: "Unapproved for e2e.mod"
	logs := feederLogs(s)
	if !strings.Contains(logs, "NotInActive") {
		t.Errorf("no NotInActive for the unapproved article (today's reason) in diablo's logs:\n%s", logs)
	}
}

// PLANNED: the tests below are for the maintainer's moderation PRs (agent-chatroom
// 20261026-112934Z). Remove the skip when the PR is in, and adjust names and texts to it.

// TestModerationApproveRequired (PLANNED, "approve" PR): readerdef "approve yes|no", default
// no: a POST with Approved: from a reader without "approve yes" gets 441; with it, the post
// goes in.
func TestModerationApproveRequired(t *testing.T) {
	t.Skip("PLANNED: readerdef approve (moderation PR 1)")
	m := newModerationReader(t, ModOptions{})
	_, line := m.postArticle(t, ModGroup, "Approved: poster@example.invalid")
	if !strings.HasPrefix(line, "441") {
		t.Errorf("Approved: without approve yes: %q, want 441", line)
	}
	m.noPost(t, "approved post without approve yes")

	access := strings.Replace(DefaultReaderAccess, "    post yes\n", "    post yes\n    approve yes\n", 1)
	m2 := newModerationReader(t, ModOptions{Access: access})
	_, line = m2.postArticle(t, ModGroup, "Approved: poster@example.invalid")
	if !strings.HasPrefix(line, "240") {
		t.Errorf("Approved: with approve yes: %q, want 240", line)
	}
	m2.waitPost(t, 1, "the approved post with approve yes")
}

// TestModerationGroupStatus (PLANNED, group-status PR): POST to a status "n" group, to an
// "x" group and to a group that doesn't exist is refused, even without checkpostgroups.
func TestModerationGroupStatus(t *testing.T) {
	t.Skip("PLANNED: group status on POST (moderation PR 2)")
	m := newModerationReader(t, ModOptions{})
	for _, g := range []string{ClosedGroup, "e2e.nonexistent"} {
		if _, line := m.postArticle(t, g); !strings.HasPrefix(line, "441") {
			t.Errorf("POST to %s: %q, want 441", g, line)
		}
	}
	m.noPost(t, "posts to closed or missing groups")
}

// TestModerationFirstModeratorOnly (PLANNED, moderator-mail PR): a crosspost to two
// moderated groups is mailed to the first one's moderator only, with the address as an
// argument to sendmail instead of -t.
func TestModerationFirstModeratorOnly(t *testing.T) {
	t.Skip("PLANNED: moderator mail (moderation PR 3)")
	m := newModerationReader(t, ModOptions{})
	if m.Mail == nil {
		t.Skip("needs ci/fake-sendmail as the system's sendmail")
	}
	_, line := m.postArticle(t, ModGroup2+","+ModGroup)
	if !strings.HasPrefix(line, "240") {
		t.Fatalf("crosspost: %q, want 240", line)
	}
	mails := m.Mail.waitMails(t, 1)
	if len(mails) != 1 {
		t.Fatalf("%d mails, want 1 (the first moderated group's moderator)", len(mails))
	}
	if to := mails[0].To(); to != "mod2@moderators.example.invalid" {
		t.Errorf("To: %q, want the first group's moderator", to)
	}
	if slices.Contains(mails[0].Argv, "-t") || !slices.Contains(mails[0].Argv, "mod2@moderators.example.invalid") {
		t.Errorf("sendmail argv %q, want the address as an argument and no -t", mails[0].Argv)
	}
}

// TestModerationNoModerator441 (PLANNED, moderator-mail PR): an unapproved POST to a
// moderated group without a moderator gets a clear 441 and is not passed on.
func TestModerationNoModerator441(t *testing.T) {
	t.Skip("PLANNED: moderator mail (moderation PR 3)")
	m := newModerationReader(t, ModOptions{})
	if _, line := m.postArticle(t, NoModGroup); !strings.HasPrefix(line, "441") {
		t.Errorf("POST to %s: %q, want 441", NoModGroup, line)
	}
	m.noPost(t, "post to a moderated group without a moderator")
}

// TestModerationMailFailureReported (PLANNED, moderator-mail PR): a failing sendmail gives
// an error reply, not "240 Article mailed".
func TestModerationMailFailureReported(t *testing.T) {
	t.Skip("PLANNED: moderator mail (moderation PR 3)")
	m := newModerationReader(t, ModOptions{})
	if m.Mail == nil {
		t.Skip("needs ci/fake-sendmail as the system's sendmail")
	}
	m.Mail.setExit(t, 75)
	if _, line := m.postArticle(t, ModGroup); strings.HasPrefix(line, "240") {
		t.Errorf("POST with a failing sendmail: %q, want an error", line)
	}
}

// TestModerationAddressFormat (PLANNED, moderator-mail PR): the moderators address is
// substituted safely: "%s" is the group, "%%" a percent sign, and anything else is
// literal (today it goes through snprintf as a format).
func TestModerationAddressFormat(t *testing.T) {
	t.Skip("PLANNED: moderator mail (moderation PR 3)")
	m := newModerationReader(t, ModOptions{Moderators: ModGroup + ":%s%%x%d%n@moderators.example.invalid\n"})
	if m.Mail == nil {
		t.Skip("needs ci/fake-sendmail as the system's sendmail")
	}
	m.postArticle(t, ModGroup)
	mails := m.Mail.waitMails(t, 1)
	if want := "e2e-mod%x%d%n@moderators.example.invalid"; len(mails) != 1 || mails[0].To() != want {
		t.Errorf("mails %v, want one To: %s", mails, want)
	}
}

// TestFeederUnapprovedReason (PLANNED, diablo PR): the unapproved article to a moderated
// group is answered and logged as "Unapproved for e2e.mod", not "NotInActive".
func TestFeederUnapprovedReason(t *testing.T) {
	t.Skip("PLANNED: diablo Unapproved reason (moderation PR 4)")
	s := NewSite(t, SiteOptions{
		Config: map[string]string{"active": "on", "activedrop": "yes", "xrefhost": PathHost, "path_generallog": "%s/general.log"},
		Setup: func(tb testing.TB, s *Site) {
			s.WriteFile(tb, "dactive.kp", "")
			s.Run(tb, "dgrpctl", "-s", "newgroup", ModGroup, "m")
		},
	})
	p := s.Peer(t)
	id := msgID(t)
	expect(t, p, 335, "IHAVE %s", id)
	send(t, p, wire(newArticle(id, ModGroup, "unapproved", "body")))
	line, _ := p.ReadLine()
	if !strings.HasPrefix(line, "437") || !strings.Contains(line, "Unapproved") {
		t.Errorf("unapproved IHAVE: %q, want 437 ... Unapproved for %s", line, ModGroup)
	}
	if !strings.Contains(feederLogs(s), "Unapproved for "+ModGroup) {
		t.Errorf("no \"Unapproved for %s\" in the log", ModGroup)
	}
}

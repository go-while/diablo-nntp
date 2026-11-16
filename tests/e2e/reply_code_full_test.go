package e2e

// dreaderd reads NNTP reply codes as the full three-digit number the command defines: no range
// ("2xx", "3xx") and no first digit, and a line that is no reply code at all ("200junk",
// " 200", "0200") is an error. These tests make a backend answer with codes the command does
// not define, or with a malformed code, and look at what dreaderd does with the connection.

import (
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	testkit "github.com/go-while/nntp-testkit"
	"github.com/go-while/nntp-testkit/client"
	"github.com/go-while/nntp-testkit/fakeserver"
)

// backendCase is a spool server ('s' in dserver.hosts) whose answers to dreaderd's set-up
// commands are replaced. dreaderd sends them in lower case ("mode reader", "authinfo user u"),
// and Faults.OverrideLine keys keep the arguments as sent.
type backendCase struct {
	name     string
	flags    string              // flags after s1p<port> in dserver.hosts ("M", "R")
	opts     string              // options after the flags ("login=u password=pw")
	greeting string              // "" = the fake's own
	lines    map[string][]string // the replies that are replaced
	ok       bool                // the backend is used: the article is fetched from it
	tag      string              // dreaderd's log tag of the step ("mode-reader", "authinfo-pass")
	fail     string              // text the step must log as an error ("" = none)
}

// backendSite starts a reader whose only spool server is a fake with the case's answers.
func backendSite(t *testing.T, c backendCase) (*Site, *fakeserver.Store, *fakeserver.Server) {
	t.Helper()
	st := fakeserver.NewStore()
	st.AddGroup("e2e.test", "e2e test group")
	srv := fakeserver.New(t, fakeserver.Options{Store: st, Greeting: c.greeting,
		Faults: fakeserver.Faults{OverrideLine: c.lines}})
	hosts := "127.0.0.1\ts1p" + strconv.Itoa(srv.Port()) + c.flags
	if c.opts != "" {
		hosts += "\t" + c.opts
	}
	s := NewReaderSite(t, ReaderOptions{Spool: srv, Files: map[string]string{"dserver.hosts": hosts + "\n"}})
	t.Cleanup(func() {
		if t.Failed() {
			t.Logf("general.log:\n%s", generalLog(s))
			t.Logf("backend got: %q", srv.Lines())
		}
	})
	return s, st, srv
}

// logHasLine: a line of the log has the tag ("mode-reader(") and the text.
func logHasLine(log, tag, text string) bool {
	for _, l := range strings.Split(log, "\n") {
		if strings.Contains(l, tag+"(") && strings.Contains(l, text) {
			return true
		}
	}
	return false
}

// backendUsed reports whether the backend got a request for an article.
func backendUsed(srv *fakeserver.Server) bool {
	for _, l := range srv.Lines() {
		f := strings.Fields(strings.ToUpper(l))
		if len(f) > 0 && (f[0] == "ARTICLE" || f[0] == "HEAD" || f[0] == "BODY" || f[0] == "STAT") {
			return true
		}
	}
	return false
}

func runBackendCases(t *testing.T, cases []backendCase) {
	for _, bc := range cases {
		t.Run(bc.name, func(t *testing.T) {
			s, st, srv := backendSite(t, bc)
			id := msgID(t)
			feedReader(t, s, st, "e2e.test", newArticle(id, "e2e.test", "reply codes", "body"))

			if bc.fail != "" {
				testkit.WaitFor(t, 10*time.Second, "the log line of "+bc.tag+" with "+strconv.Quote(bc.fail), func() bool {
					return logHasLine(generalLog(s), bc.tag, bc.fail)
				})
			}
			c := s.Reader(t)
			defer c.Close()
			if bc.ok {
				c.Deadline = 20 * time.Second
				c.ExpectCmd(211, "GROUP e2e.test")
				c.ExpectCmd(220, "ARTICLE %s", id)
				c.ReadDotLines()
				if bc.fail == "" && (logHasLine(generalLog(s), bc.tag, "failed: ") || logHasLine(generalLog(s), bc.tag, "unrecognized")) {
					t.Errorf("the log reports a failure of %s for an answer that is good", bc.tag)
				}
				return
			}
			// the backend is not used: dreaderd never asks it for the article
			c.Deadline = 4 * time.Second
			c.ExpectCmd(211, "GROUP e2e.test")
			if code, line, err := c.TryCmd("ARTICLE %s", id); err == nil && code == 220 {
				t.Errorf("ARTICLE was served (%q) from a backend that must not be used", line)
			}
			if backendUsed(srv) {
				t.Errorf("dreaderd asked the backend for an article: %q", srv.Lines())
			}
		})
	}
}

// TestBackendModeReaderCode: MODE READER is answered with 200 or 201 (RFC 3977 5.3), nothing
// else. Before, any 2xx ("250", "281") and any line that strtol() read as 200 (" 200", "0200",
// "200junk") made the backend usable.
func TestBackendModeReaderCode(t *testing.T) {
	mr := func(answer string) map[string][]string { return map[string][]string{"MODE reader": {answer}} }
	runBackendCases(t, []backendCase{
		{name: "200", flags: "M", lines: mr("200 ok"), ok: true, tag: "mode-reader"},
		{name: "201", flags: "M", lines: mr("201 ok"), ok: true, tag: "mode-reader"},
		{name: "250", flags: "M", lines: mr("250 x"), tag: "mode-reader", fail: "failed: 250 x"},
		{name: "281", flags: "M", lines: mr("281 x"), tag: "mode-reader", fail: "failed: 281 x"},
		{name: "leading-space", flags: "M", lines: mr(" 200 x"), tag: "mode-reader", fail: "failed:  200 x"},
		{name: "leading-zero", flags: "M", lines: mr("0200 x"), tag: "mode-reader", fail: "failed: 0200 x"},
		{name: "glued-text", flags: "M", lines: mr("200junk"), tag: "mode-reader", fail: "failed: 200junk"},
	})
}

// TestBackendModeReadonlyCode: MODE READONLY (flag R, the Diablo feeder's command) is good
// with 250, which the Diablo feeder answers; the backend is used either way, but any other
// answer is logged as a failure. Before, every 2xx counted as good.
func TestBackendModeReadonlyCode(t *testing.T) {
	ro := func(answer string) map[string][]string { return map[string][]string{"MODE readonly": {answer}} }
	runBackendCases(t, []backendCase{
		{name: "250", flags: "R", lines: ro("250 Mode Command OK."), ok: true, tag: "mode-readonly"},
		{name: "299", flags: "R", lines: ro("299 x"), ok: true, tag: "mode-readonly", fail: "failed: 299 x"},
		{name: "200", flags: "R", lines: ro("200 x"), ok: true, tag: "mode-readonly", fail: "failed: 200 x"},
	})
}

// TestBackendAuthinfoCode: dreaderd logs in to a backend with login= and password=.
// AUTHINFO USER: 381 sends the password, 281 is an account without a password and is used as
// it is (RFC 4643 2.3), anything else ends the connection. AUTHINFO PASS: 281 only. Before:
// USER 281 ended the connection, USER 3xx and PASS 2xx were taken for good.
func TestBackendAuthinfoCode(t *testing.T) {
	au := func(user, pass string) map[string][]string {
		m := map[string][]string{"AUTHINFO user u": {user}}
		if pass != "" {
			m["AUTHINFO pass pw"] = []string{pass}
		}
		return m
	}
	const lg = "login=u password=pw"
	runBackendCases(t, []backendCase{
		{name: "381-281", opts: lg, lines: au("381 pw", "281 ok"), ok: true, tag: "authinfo-pass"},
		{name: "user-281", opts: lg, lines: au("281 ok", ""), ok: true, tag: "authinfo-user"},
		{name: "pass-250", opts: lg, lines: au("381 pw", "250 x"), tag: "authinfo-pass", fail: "failed: 250 x"},
		{name: "pass-200", opts: lg, lines: au("381 pw", "200 x"), tag: "authinfo-pass", fail: "failed: 200 x"},
		{name: "pass-281-glued", opts: lg, lines: au("381 pw", "281junk"), tag: "authinfo-pass", fail: "failed: 281junk"},
		{name: "user-399", opts: lg, lines: au("399 x", "281 ok"), tag: "authinfo-user", fail: "failed: 399 x"},
		{name: "user-leading-space", opts: lg, lines: au(" 381 x", "281 ok"), tag: "authinfo-user", fail: "failed:  381 x"},
	})
}

// TestBackendUserWithoutPassword: after a 281 to AUTHINFO USER dreaderd does not send the
// password, and the backend is used.
func TestBackendUserWithoutPassword(t *testing.T) {
	s, st, srv := backendSite(t, backendCase{opts: "login=u password=pw",
		lines: map[string][]string{"AUTHINFO user u": {"281 no password needed"}}})
	id := msgID(t)
	feedReader(t, s, st, "e2e.test", newArticle(id, "e2e.test", "no password", "body"))
	c := s.Reader(t)
	defer c.Close()
	c.Deadline = 20 * time.Second
	c.ExpectCmd(211, "GROUP e2e.test")
	c.ExpectCmd(220, "ARTICLE %s", id)
	c.ReadDotLines()
	for _, l := range srv.Lines() {
		if strings.HasPrefix(strings.ToLower(l), "authinfo pass") {
			t.Errorf("dreaderd sent the password after a 281 to AUTHINFO USER: %q", srv.Lines())
		}
	}
}

// TestBackendGreetingCode: the backend's greeting is 200 or 201 as three digits and then a
// space or the end of the line. Before, " 200 x", "0200 x" and "200junk" were read as 200.
func TestBackendGreetingCode(t *testing.T) {
	runBackendCases(t, []backendCase{
		{name: "200", greeting: "200 ready", ok: true, tag: "connect"},
		{name: "201", greeting: "201 ready (no posting)", ok: true, tag: "connect"},
		{name: "leading-space", greeting: " 200 x", tag: "connect", fail: "unrecognized banner:  200 x"},
		{name: "leading-zero", greeting: "0200 x", tag: "connect", fail: "unrecognized banner: 0200 x"},
		{name: "glued-text", greeting: "200junk", tag: "connect", fail: "unrecognized banner: 200junk"},
	})
}

// loginOK logs in as user and says whether AUTHINFO PASS got 281.
func loginOK(t *testing.T, s *Site, user, pass string) bool {
	t.Helper()
	c := client.DialWith(t, s.Addr, 2*authTimeoutBound)
	defer c.Close()
	c.ExpectCmd(381, "AUTHINFO USER %s", user)
	code, line, err := c.TryCmd("AUTHINFO PASS %s", pass)
	t.Logf("%s: %d %q %v", user, code, line, err)
	return err == nil && code == 281
}

// TestReaderPerlAuthFullCode: the answer of a Perl checkuser() logs the user in only when it is
// "100 ..." or "110 <readerdef>" as a whole code. Before, any answer that started with the
// digit 1 did ("199 no", "1", "1garbage", "100x", "110" without a readerdef, "120 x").
// Skipped when dreaderd is built without Perl support.
func TestReaderPerlAuthFullCode(t *testing.T) {
	builtWith(t, "PERL auth request", "Perl support (PERL_ENABLED)")
	cases := []struct {
		user, answer string
		ok           bool
	}{
		{"ok", "100 Success", true},
		{"rdef", "110 100mbit", true},
		{"n199", "199 no", false},
		{"one", "1", false},
		{"onejunk", "1garbage", false},
		{"c100x", "100x", false},
		{"c110", "110", false},
		{"c120", "120 x", false},
	}
	var m strings.Builder
	for _, tc := range cases {
		fmt.Fprintf(&m, "    \"%s\" => \"%s\",\n", tc.user, tc.answer)
	}
	script := "my %ans = (\n" + m.String() + ");\n" + `sub checkuser {
    my ($user, $pass) = @_;
    return "0" unless $pass eq "pw" && exists $ans{$user};
    return $ans{$user};
}
1;
`
	s := refusalSiteWith(t, map[string]string{"auth.pl": script}, "perl", "auth.pl")
	for _, tc := range cases {
		if got := loginOK(t, s, tc.user, "pw"); got != tc.ok {
			t.Errorf("the Perl answer %q: logged in = %v, want %v", tc.answer, got, tc.ok)
		}
	}
}

// netRemoteAnswering is a NetRemote server that reads the whole request of a connection (it
// cannot decrypt it) and answers with the line that set() gave last.
func netRemoteAnswering(t *testing.T) (port int, set func(answer string)) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	var mu sync.Mutex
	answer := ""
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				c.SetDeadline(time.Now().Add(10 * time.Second))
				if _, err := io.WriteString(c, "000000000000\n"); err != nil {
					return
				}
				hdr := make([]byte, 13)
				if _, err := io.ReadFull(c, hdr); err != nil {
					return
				}
				size, err := strconv.Atoi(strings.TrimSpace(string(hdr)))
				if err != nil || size <= 0 || size > 65536 {
					return
				}
				if _, err := io.ReadFull(c, make([]byte, size)); err != nil {
					return
				}
				mu.Lock()
				a := answer
				mu.Unlock()
				io.WriteString(c, a)
			}(c)
		}
	}()
	return ln.Addr().(*net.TCPAddr).Port, func(a string) {
		mu.Lock()
		answer = a
		mu.Unlock()
	}
}

// TestNetRemoteAnswerFullCode: a NetRemote server's answer logs the user in when it is "1" (the
// one-digit protocol), "100 ..." or "110 <readerdef>"; "0" and anything else that starts with
// 1 is a refusal. Before, any first character 1 counted ("1garbage", "12", "199 no").
// Skipped when dreaderd is built without NetRemote support.
func TestNetRemoteAnswerFullCode(t *testing.T) {
	builtWith(t, "NetRemote socket connect", "NetRemote support (NETREMOTE_ENABLED in lib/vendor.h)")
	port, set := netRemoteAnswering(t)
	s := authTimeoutSite(t, slowAuthAccess("netremote secret@127.0.0.1:"+strconv.Itoa(port)))
	for i, tc := range []struct {
		answer string
		ok     bool
	}{
		{"1\n", true},
		{"100 Success\n", true},
		{"100 Success\r\n", true},
		{"0\n", false},
		{"1garbage\n", false},
		{"12\n", false},
		{"1 x\n", false},
		{"199 no\n", false},
		{"110\n", false},
	} {
		set(tc.answer)
		user := fmt.Sprintf("u%d@slow", i)
		if got := loginOK(t, s, user, "pw"); got != tc.ok {
			t.Errorf("the NetRemote answer %q: logged in = %v, want %v", tc.answer, got, tc.ok)
		}
	}
}

// ihaveBackend is the outgoing server ('o' in dserver.hosts): it answers IHAVE with ihave and,
// when the article arrives, with end. n() counts the articles it got whole.
func ihaveBackend(t *testing.T, ihave, end string) (*fakeserver.Server, func() int) {
	t.Helper()
	var n atomic.Int32
	srv := fakeserver.New(t, fakeserver.Options{Handler: fakeserver.LineHandler(
		func(c *fakeserver.Conn, line string) ([]string, bool) {
			f := strings.Fields(line)
			if len(f) == 0 {
				return nil, false
			}
			switch strings.ToUpper(f[0]) {
			case "IHAVE":
				if c.WriteLines(ihave) != nil {
					return nil, true
				}
				// dreaderd sends the article when it takes the reply for a 335;
				// otherwise it closes the connection and this read ends
				if _, err := c.ReadDotBlock(); err != nil {
					return nil, true
				}
				n.Add(1)
				return []string{end}, false
			case "QUIT":
				return []string{"205 bye"}, true
			}
			return []string{"500 unexpected"}, false
		})})
	return srv, func() int { return int(n.Load()) }
}

// postVia POSTs an article to a reader whose outgoing server is post, and returns the reply
// code (0 when none came within wait).
func postVia(t *testing.T, post *fakeserver.Server, wait time.Duration) (int, string) {
	t.Helper()
	_, spool := spoolStore(t)
	s := NewReaderSite(t, ReaderOptions{Spool: spool, Post: post})
	t.Cleanup(func() {
		if t.Failed() {
			t.Logf("general.log:\n%s", generalLog(s))
		}
	})
	c := s.Reader(t)
	defer c.Close()
	c.ExpectCmd(340, "POST")
	c.MustSend(client.Stuff([]string{
		"From: E2E Poster <poster@example.invalid>",
		"Newsgroups: e2e.test",
		"Subject: reply codes",
		"Message-ID: " + msgID(t),
		"",
		"body",
	}))
	c.Deadline = wait
	code, line, err := c.ReadStatus()
	if err != nil {
		return 0, err.Error()
	}
	return code, line
}

// TestPostBackendReplyCodes: the outgoing server must answer IHAVE with 335 and the article
// with 235 as whole codes. A line that is no reply code ("335junk", " 235") ends the connection
// (the article is queued again), it is not taken for 335 or 235. Before, strtol() read both,
// and the client got "240 Article posted". Another valid code still gives "441" with the
// server's line.
func TestPostBackendReplyCodes(t *testing.T) {
	for _, tc := range []struct {
		name, ihave, end string
		want             int // the client's reply code; 0 = none: the post is not taken for done
		sent             bool
	}{
		{"good", "335 go", "235 ok", 240, true},
		{"ihave-435", "435 have it", "235 ok", 441, false},
		{"article-437", "335 go", "437 rejected", 441, true},
		{"ihave-glued", "335junk", "235 ok", 0, false},
		{"ihave-leading-space", " 335 go", "235 ok", 0, false},
		{"article-glued", "335 go", "235junk", 0, true},
		{"article-leading-space", "335 go", " 235 ok", 0, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			post, n := ihaveBackend(t, tc.ihave, tc.end)
			code, line := postVia(t, post, 6*time.Second)
			if code != tc.want {
				t.Errorf("POST: reply %d %q, want %d", code, line, tc.want)
			}
			if tc.sent && n() == 0 {
				t.Errorf("the outgoing server got no article")
			}
			if !tc.sent && n() != 0 {
				t.Errorf("the outgoing server got an article after the reply %q to IHAVE", tc.ihave)
			}
		})
	}
}

// TestSpoolArticleReplyCode: the spool server's answer to ARTICLE is 220 as a whole code. Before,
// strtol() read "220junk" and " 220 x" as 220 and the client got the lines as an article. They are
// not an article now. The lines after such a reply would be read as the reply to the next
// request on the connection: dreaderd closes it, so a HEAD that follows gets its own 221.
func TestSpoolArticleReplyCode(t *testing.T) {
	for _, tc := range []struct{ name, answer string }{
		{"glued-text", "220junk 1 <x@e2e.test>"},
		{"leading-space", " 220 1 <x@e2e.test>"},
	} {
		answer := tc.answer
		t.Run(tc.name, func(t *testing.T) {
			st := fakeserver.NewStore()
			st.AddGroup("e2e.test", "e2e test group")
			id := msgID(t)
			srv := fakeserver.New(t, fakeserver.Options{Store: st, Faults: fakeserver.Faults{
				OverrideLine: map[string][]string{"ARTICLE " + id: {answer, "Subject: x", "", "body", "."}}}})
			s := NewReaderSite(t, ReaderOptions{Spool: srv})
			feedReader(t, s, st, "e2e.test", newArticle(id, "e2e.test", "reply codes", "body"))
			c := s.Reader(t)
			defer c.Close()
			c.Deadline = 10 * time.Second
			c.ExpectCmd(211, "GROUP e2e.test")
			code, line, err := c.TryCmd("ARTICLE %s", id)
			if err == nil && code == 220 {
				t.Errorf("a spool answer %q was taken for 220: the client got %q", answer, line)
			}
			if !backendUsed(srv) {
				t.Errorf("dreaderd did not ask the spool server for the article")
			}
			// the next request is not answered by the lines left over from the bad reply
			c.ExpectCmd(221, "HEAD %s", id)
			c.ReadDotLines()
		})
	}
}

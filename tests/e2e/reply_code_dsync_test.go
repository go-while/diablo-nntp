package e2e

// Reply codes in dsyncgroups: the whole three-digit code, compared with the codes the command
// defines; 281 to AUTHINFO USER is a login, an unknown reply ends the session.

import (
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/go-while/nntp-testkit/fakeserver"
)

// rcuSyncPeer is a remote end for dsyncgroups: user is the answer to AUTHINFO USER, mode the one
// to MODE READER; AUTHINFO PASS gets 502 (RFC 4643: after a login), LIST ACTIVE one group.
func rcuSyncPeer(t *testing.T, user, mode []string) *fakeserver.Server {
	t.Helper()
	return fakeserver.New(t, fakeserver.Options{Greeting: "200 fake", Handler: fakeserver.LineHandler(func(_ *fakeserver.Conn, line string) ([]string, bool) {
		l := strings.ToLower(strings.TrimSpace(line))
		switch {
		case strings.HasPrefix(l, "authinfo user"):
			return user, false
		case strings.HasPrefix(l, "authinfo pass"):
			return []string{"502 already logged in"}, false
		case strings.HasPrefix(l, "mode reader"):
			return mode, false
		case strings.HasPrefix(l, "list active"):
			return []string{"215 list follows", "alpha 10 1 y", "."}, false
		case strings.HasPrefix(l, "quit"):
			return []string{"205 bye"}, true
		}
		return []string{"500 what?"}, false
	})})
}

// TestDsyncgroupsAuthinfoUser281: a server may answer AUTHINFO USER with 281, a login that needs
// no password (RFC 4643 2.3). dsyncgroups then is logged in and sends no AUTHINFO PASS, also
// when it has a password (-u). Fails on main: only 381 was taken, 281 gave "authinfo denied"
// and exit 1.
func TestDsyncgroupsAuthinfoUser281(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
	}{
		{"user only", []string{"-U", "fred"}},
		{"user and password", []string{"-U", "fred", "-u", "secret"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := NewSite(t, SiteOptions{NoStart: true})
			srv := rcuSyncPeer(t, []string{"281 no password needed"}, []string{"200 reader"})
			a := append([]string{"-h", "127.0.0.1", "-p", strconv.Itoa(srv.Port()),
				"-F", filepath.Join(s.Dir, "dactive.kp"), "-z"}, tc.args...)
			out, err := s.TryRun("dsyncgroups", a...)
			if err != nil || !strings.Contains(out, "1 groups returned by list active") {
				t.Errorf("dsyncgroups after 281 to AUTHINFO USER: err=%v, want the group list:\n%s", err, out)
			}
			for _, l := range srv.Lines() {
				if strings.HasPrefix(strings.ToLower(l), "authinfo pass") {
					t.Errorf("AUTHINFO PASS sent after 281 to AUTHINFO USER (lines %q)", srv.Lines())
				}
			}
		})
	}
}

// TestDsyncgroupsModeReaderReply: a one-line refusal of MODE READER (501) lets dsyncgroups go on
// as before; a code MODE READER does not have (280) ends the session with exit 1, since lines
// that come with it would be read as the reply to LIST ACTIVE. Fails on main for 280: every
// reply but 200/201 went on ("Going on anyway").
func TestDsyncgroupsModeReaderReply(t *testing.T) {
	for _, tc := range []struct {
		reply string
		ok    bool
	}{
		{"501 unknown mode", true},
		{"280 odd", false},
	} {
		t.Run(tc.reply, func(t *testing.T) {
			s := NewSite(t, SiteOptions{NoStart: true})
			srv := rcuSyncPeer(t, nil, []string{tc.reply})
			out, err := s.TryRun("dsyncgroups", "-h", "127.0.0.1", "-p", strconv.Itoa(srv.Port()),
				"-F", filepath.Join(s.Dir, "dactive.kp"), "-z")
			if tc.ok && (err != nil || !strings.Contains(out, "1 groups returned by list active")) {
				t.Errorf("dsyncgroups after %q to MODE READER: err=%v, want it to go on:\n%s", tc.reply, err, out)
			}
			if !tc.ok && (err == nil || !strings.Contains(out, "Unexpected reply to mode reader")) {
				t.Errorf("dsyncgroups after %q to MODE READER: err=%v, want exit 1 at MODE READER:\n%s", tc.reply, err, out)
			}
		})
	}
}

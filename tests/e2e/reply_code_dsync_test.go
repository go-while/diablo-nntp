package e2e

// Reply codes in dsyncgroups and dclient: the whole three-digit code, compared with the codes the
// command defines; 281 to AUTHINFO USER is a login, an unknown reply ends the session.

import (
	"errors"
	"os"
	"os/exec"
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

// TestDsyncgroupsAuthinfoUser381NoPassword: 381 to AUTHINFO USER asks for a password; without one
// (-u) dsyncgroups ends the session (exit 1) and sends no AUTHINFO PASS.
func TestDsyncgroupsAuthinfoUser381NoPassword(t *testing.T) {
	s := NewSite(t, SiteOptions{NoStart: true})
	srv := rcuSyncPeer(t, []string{"381 password please"}, []string{"200 reader"})
	out, err := s.TryRun("dsyncgroups", "-h", "127.0.0.1", "-p", strconv.Itoa(srv.Port()),
		"-F", filepath.Join(s.Dir, "dactive.kp"), "-z", "-U", "fred")
	if err == nil || !strings.Contains(out, "authinfo wants a password") {
		t.Errorf("dsyncgroups after 381 without a password: err=%v, want exit 1 with \"authinfo wants a password\":\n%s", err, out)
	}
	for _, l := range srv.Lines() {
		if strings.HasPrefix(strings.ToLower(l), "authinfo pass") || strings.HasPrefix(strings.ToLower(l), "list") {
			t.Errorf("%q sent after 381 without a password (lines %q)", l, srv.Lines())
		}
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

// TestDclientReplyCodes: dclient reads a reply code as the whole number. A reply without one
// stops it (exit 2): it cannot tell whether lines follow. QUIT is answered by 205 only, also a
// bare "205" with no text; a bare "200" greeting is a greeting. Fails on main: atoi() took the
// line without a code as 0, a reply without lines, and dclient went on (exit 0); the bare "205"
// and the bare "200" were refused (exit 2).
func TestDclientReplyCodes(t *testing.T) {
	for _, tc := range []struct {
		name, greeting, date, quit string
		exit                       int
		out                        string
	}{
		{"reply without a code", "200 fake", "x no code here", "205 bye", 2, "Unexpected response from server"},
		{"bare 205 to QUIT", "200 fake", "111 20261116000000", "205", 0, "111 20261116000000"},
		{"bare 200 greeting", "200", "111 20261116000000", "205 bye", 0, "111 20261116000000"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := NewSite(t, SiteOptions{NoStart: true})
			srv := fakeserver.New(t, fakeserver.Options{Greeting: tc.greeting, Handler: fakeserver.LineHandler(func(_ *fakeserver.Conn, line string) ([]string, bool) {
				l := strings.ToLower(strings.TrimSpace(line))
				switch {
				case strings.HasPrefix(l, "date"):
					return []string{tc.date}, false
				case strings.HasPrefix(l, "quit"):
					return []string{tc.quit}, true
				}
				return []string{"500 what?"}, false
			})})
			out, err := s.TryRun("dclient", "-p", strconv.Itoa(srv.Port()), "127.0.0.1", "DATE")
			code := 0
			var ee *exec.ExitError
			if errors.As(err, &ee) {
				code = ee.ExitCode()
			} else if err != nil {
				t.Fatalf("dclient: %v\n%s", err, out)
			}
			if code != tc.exit || !strings.Contains(out, tc.out) {
				t.Errorf("dclient DATE: exit %d, want %d, and output %q, want %q in it", code, tc.exit, out, tc.out)
			}
		})
	}
}

// TestDclientDumpFileQuit: dclient -f sends the commands of a file without reading their replies;
// at QUIT it skips those replies up to the reply to QUIT (205), so a working session ends with
// exit 0. Fails on main for DATE: the first reply read at QUIT was the 111 to DATE, refused
// (exit 2); main took only 238 (a CHECK) there as a stand-in.
func TestDclientDumpFileQuit(t *testing.T) {
	for _, tc := range []struct{ name, cmds string }{
		{"CHECK", "CHECK <dump1@e2e.test>\nCHECK <dump2@e2e.test>\n"},
		{"DATE", "DATE\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := NewSite(t, SiteOptions{NoStart: true})
			srv := fakeserver.New(t, fakeserver.Options{Greeting: "200 fake", Handler: fakeserver.LineHandler(func(_ *fakeserver.Conn, line string) ([]string, bool) {
				l := strings.ToLower(strings.TrimSpace(line))
				f := strings.Fields(line)
				switch {
				case strings.HasPrefix(l, "check ") && len(f) > 1:
					return []string{"238 " + f[1]}, false
				case strings.HasPrefix(l, "date"):
					return []string{"111 20261116000000"}, false
				case strings.HasPrefix(l, "quit"):
					return []string{"205 bye"}, true
				}
				return []string{"500 what?"}, false
			})})
			file := filepath.Join(s.Dir, "dump.txt")
			if err := os.WriteFile(file, []byte(tc.cmds), 0o644); err != nil {
				t.Fatal(err)
			}
			out, err := s.TryRun("dclient", "-f", file, "-p", strconv.Itoa(srv.Port()), "127.0.0.1")
			if err != nil {
				t.Errorf("dclient -f with %q: %v, want exit 0 after the 205 to QUIT:\n%s", tc.cmds, err, out)
			}
		})
	}
}

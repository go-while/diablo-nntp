package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestReaderAuthinfoNotLogged: with logcmd yes, dreaderd logs every command but never the
// password of AUTHINFO PASS, however the words are separated (the parser accepts any run of
// spaces and tabs) and in any case, and not the credentials of an AUTHINFO SASL either.
// Before the fix only a line starting with exactly "authinfo pass " was masked.
func TestReaderAuthinfoNotLogged(t *testing.T) {
	const pass = "Pw-e2e-n0t-in-log"
	const sasl = "AGpvZQBQdy1lMmUtbjB0LWluLWxvZw==" // SASL PLAIN initial response: user and password
	access := `vserverdef DEFAULT
    hostname ` + PathHost + `
end
authdef joe
    user joe
    pass ` + pass + `
end
readerdef userjoe
    read yes
    auth joe
    logcmd yes
end
access 127.0.0.1 userjoe
`
	_, spool := spoolStore(t)
	s := NewReaderSite(t, ReaderOptions{Spool: spool, Access: access,
		Config: map[string]string{"path_generallog": "%s/general.log"}})

	logins := [][2]string{
		{"AUTHINFO USER joe", "AUTHINFO  PASS " + pass},
		{"authinfo\tuser joe", "AUTHINFO\tPASS\t" + pass},
		{"AuthInfo   User joe", "Authinfo \t pass " + pass},
		{"AUTHINFO USER joe", "AUTHINFO PASS " + pass},
	}
	for _, l := range logins {
		c := s.Reader(t)
		expect(t, c, 381, "%s", l[0])
		expect(t, c, 281, "%s", l[1])
		expect(t, c, 205, "QUIT")
	}
	c := s.Reader(t)
	expect(t, c, 501, "AUTHINFO  SASL PLAIN %s", sasl)
	expect(t, c, 205, "QUIT")

	logged := s.ReadFile(t, "log/general.log")
	if n := strings.Count(logged, "authinfo pass **unlogged**"); n < len(logins) {
		t.Errorf("general.log has %d masked AUTHINFO PASS lines, want %d (logcmd not working?)", n, len(logins))
	}
	for _, secret := range []string{pass, sasl} {
		if strings.Contains(logged, secret) {
			for _, line := range strings.Split(logged, "\n") {
				if strings.Contains(line, secret) {
					t.Errorf("general.log shows a secret: %q", line)
				}
			}
		}
	}
	if b, err := os.ReadFile(filepath.Join(s.Dir, "log", "dreaderd.status")); err == nil && strings.Contains(string(b), pass) {
		t.Errorf("dreaderd.status shows the password")
	}
}

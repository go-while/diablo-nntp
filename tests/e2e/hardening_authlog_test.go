package e2e

import (
	"strings"
	"testing"
	"time"
)

// TestReaderLogcmdMasksPassword: with `logcmd yes`, the password of AUTHINFO PASS is not
// written to general.log, whatever run of spaces or tabs separates the words.
func TestReaderLogcmdMasksPassword(t *testing.T) {
	_, spool := spoolStore(t)
	access := strings.Replace(AuthReaderAccess, "    auth fred\n", "    auth fred\n    logcmd yes\n", 1)
	s := NewReaderSite(t, ReaderOptions{Spool: spool, Access: access,
		Config: map[string]string{"path_generallog": "%s/general.log"}})

	cmds := []string{
		"AUTHINFO PASS pwplain-secret1",
		"authinfo pass pwlower-secret2",
		"AUTHINFO  PASS pwspace-secret3",
		"AUTHINFO\tPASS pwtab-secret4",
		"AUTHINFO \t PASS  pwmixed-secret5",
	}
	for _, cmd := range cmds {
		c := s.Reader(t)
		expect(t, c, 381, "AUTHINFO USER fred")
		expect(t, c, 481, "%s", cmd)
	}
	time.Sleep(time.Second)
	log := s.ReadFile(t, "log/general.log")
	for i := 1; i <= len(cmds); i++ {
		if strings.Contains(log, "-secret"+string(rune('0'+i))) {
			t.Errorf("password of %q written to the log", cmds[i-1])
		}
	}
	if !strings.Contains(log, "AUTHINFO USER fred") && !strings.Contains(log, "authinfo user fred") {
		t.Errorf("logcmd did not log the other commands")
	}
}

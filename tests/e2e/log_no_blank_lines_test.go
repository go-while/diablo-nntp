package e2e

import (
	"strings"
	"testing"
	"time"

	testkit "github.com/go-while/nntp-testkit"
)

// TestLogNoBlankLines: a log message whose format ends in a newline of its own (here the
// "Invalid vserverdef option ... not installed\n" of dreaderd's access file check; the Perl
// auth result line is another) is written as one line, without a blank line after it. Fails
// on main: the log writer added its newline to the message's, so general.log had an empty
// line after each such message.
func TestLogNoBlankLines(t *testing.T) {
	access := strings.Replace(DefaultReaderAccess, "    hostname "+PathHost+"\n",
		"    hostname "+PathHost+"\n    nosuchoption yes\n", 1)
	s := NewReaderSite(t, ReaderOptions{SpoolAddr: "127.0.0.1:1", NoStart: true, Access: access,
		Config: map[string]string{"path_generallog": "%s/general.log"}})
	startReaderNoWait(t, s)

	testkit.WaitFor(t, 10*time.Second, "the invalid option line in general.log", func() bool {
		return strings.Contains(generalLog(s), "Invalid vserverdef option")
	})
	g := generalLog(s)
	for i, l := range strings.Split(strings.TrimSuffix(g, "\n"), "\n") {
		if strings.TrimSpace(l) == "" {
			t.Errorf("general.log line %d is empty:\n%s", i+1, g)
			break
		}
	}
}

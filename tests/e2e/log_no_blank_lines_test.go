package e2e

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	testkit "github.com/go-while/nntp-testkit"
)

// TestLogNoBlankLines: a log message whose format ends in a newline of its own (here the
// "Invalid vserverdef option ... not installed\n" of dreaderd's access file check; the Perl
// auth result line is another) is written as one line, without a blank line after it. Fails
// on main: the log writer added its newline to the message's, so general.log had an empty
// line after each such message. A CR or LF inside a message (here a Perl auth result, which is
// script output) becomes a space, so the entry stays on one line.
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

	t.Run("perl result with line breaks", func(t *testing.T) {
		b, err := os.ReadFile(filepath.Join(bin(t), "dreaderd"))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Contains(b, []byte("PERL auth request")) {
			t.Skip("dreaderd is built without Perl support (PERL_ENABLED)")
		}
		script := filepath.Join(t.TempDir(), "auth.pl")
		if err := os.WriteFile(script, []byte("sub checkuser {\n    return \"0 refused\\r\\nsecond line\\n\";\n}\n1;\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		access := "vserverdef DEFAULT\n    hostname " + PathHost + "\nend\nauthdef viaperl\n    perl " + script +
			"\nend\nreaderdef userpat\n    read yes\n    auth viaperl\nend\nreaderdef feed\n    feed yes\nend\n" +
			"access 127.0.0.1 userpat\naccess " + PeerIP + " feed\n"
		_, spool := spoolStore(t)
		s := NewReaderSite(t, ReaderOptions{Spool: spool, Access: access,
			Config: map[string]string{"path_generallog": "%s/general.log"}})
		c := s.Reader(t)
		c.ExpectCmd(381, "AUTHINFO USER pat")
		c.Cmd("AUTHINFO PASS x")
		testkit.WaitFor(t, 10*time.Second, "the Perl auth line in general.log", func() bool {
			return strings.Contains(generalLog(s), "PERL auth request for pat")
		})
		g := generalLog(s)
		for _, l := range strings.Split(strings.TrimSuffix(g, "\n"), "\n") {
			if strings.Contains(l, "PERL auth request for pat") && !strings.Contains(l, "0 refused  second line") {
				t.Errorf("the Perl result is not on one line: %q\n%s", l, g)
			}
			if strings.TrimSpace(l) == "" || strings.HasPrefix(l, "second line") {
				t.Errorf("general.log has a split or empty line %q:\n%s", l, g)
			}
		}
	})
}

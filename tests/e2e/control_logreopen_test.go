package e2e

import (
	"slices"
	"strings"
	"testing"
	"time"
)

// TestControlLogRuleReopenFailure: a dcontrol.ctl rule with two log= actions where the
// second log file cannot be opened (its directory does not exist) must not take the daemon
// down. The first log= opens a file and sets the control log handle; the second closes it
// and then fails to reopen. The closed handle has to be forgotten so the temporary-file
// fallback runs, every later write to the control log goes to a valid file, and the handle
// is not closed a second time when the control is freed. The control is still executed and
// the reader stays responsive afterwards.
func TestControlLogRuleReopenFailure(t *testing.T) {
	st, spool := spoolStore(t)
	s := NewReaderSite(t, ReaderOptions{
		Spool: spool,
		// log=good.log opens under the site log dir; log=noexist/bad.log cannot be
		// created because "noexist" is not an existing directory, so its open fails.
		Files: map[string]string{"dcontrol.ctl": "newgroup:*:*:doit,log=good.log,log=noexist/bad.log\n"},
	})
	a := newArticle(msgID(t), "e2e.test", "cmsg newgroup e2e.logreopen", "For your newsgroups file:", "e2e.logreopen\tA test group.")
	for i, l := range a {
		if l == "" {
			a = append(append(append([]string{}, a[:i]...), "Control: newgroup e2e.logreopen"), a[i:]...)
			break
		}
	}
	exists := func(g string) bool {
		c := s.Reader(t)
		defer expect(t, c, 205, "QUIT")
		expect(t, c, 215, "LIST ACTIVE %s", g)
		return slices.ContainsFunc(c.ReadDotLines(), func(l string) bool { return strings.HasPrefix(l, g+" ") })
	}

	feedReader(t, s, st, "e2e.test", a)
	waitFor(t, 10*time.Second, "e2e.logreopen created despite the failed second log file", func() bool { return exists("e2e.logreopen") })

	// The reader is still alive: a second control message is processed too.
	b := newArticle(msgID(t), "e2e.test", "cmsg newgroup e2e.logreopen2", "For your newsgroups file:", "e2e.logreopen2\tAnother test group.")
	for i, l := range b {
		if l == "" {
			b = append(append(append([]string{}, b[:i]...), "Control: newgroup e2e.logreopen2"), b[i:]...)
			break
		}
	}
	feedReader(t, s, st, "e2e.test", b)
	waitFor(t, 10*time.Second, "e2e.logreopen2 created by a second control on a healthy reader", func() bool { return exists("e2e.logreopen2") })
}

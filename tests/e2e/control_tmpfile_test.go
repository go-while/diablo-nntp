package e2e

import (
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"
)

// TestControlMailRuleTempFile: a dcontrol.ctl rule with "mail" and no log= makes dreaderd keep
// the control log in a temporary file, which it creates in /tmp and removes when it is done.
// The control message is still executed, and no dreaderd.ctl temporary file of this site's
// processes is left behind. (The file name used to be the fixed /tmp/dreaderd.ctl<pid>, which
// a local user could replace with a symlink between remove() and open(); it is now made with
// mkstemp(), GHSA-7prm-44hq-cpq3. The race itself cannot be hit deterministically.)
func TestControlMailRuleTempFile(t *testing.T) {
	st, spool := spoolStore(t)
	s := NewReaderSite(t, ReaderOptions{
		Spool: spool,
		Files: map[string]string{"dcontrol.ctl": "newgroup:*:*:doit,mail\n"},
	})
	a := newArticle(msgID(t), "e2e.test", "cmsg newgroup e2e.mailrule", "For your newsgroups file:", "e2e.mailrule\tA test group.")
	for i, l := range a {
		if l == "" {
			a = append(append(append([]string{}, a[:i]...), "Control: newgroup e2e.mailrule"), a[i:]...)
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
	waitFor(t, 10*time.Second, "e2e.mailrule created by a control message under a mail rule", func() bool { return exists("e2e.mailrule") })

	time.Sleep(500 * time.Millisecond)
	for _, pid := range sitePids(s, "dreaderd") {
		left, _ := filepath.Glob("/tmp/dreaderd.ctl" + strconv.Itoa(pid) + "*")
		for _, f := range left {
			if fi, err := os.Lstat(f); err == nil {
				t.Errorf("temporary control file left behind: %s (%v)", f, fi.Mode())
			}
		}
	}
}

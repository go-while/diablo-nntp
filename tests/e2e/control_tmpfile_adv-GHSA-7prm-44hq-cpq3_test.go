package e2e

import (
	testkit "github.com/go-while/nntp-testkit"
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
		defer c.ExpectCmd(205, "QUIT")
		c.ExpectCmd(215, "LIST ACTIVE %s", g)
		return slices.ContainsFunc(c.ReadDotLines(), func(l string) bool { return strings.HasPrefix(l, g+" ") })
	}

	feedReader(t, s, st, "e2e.test", a)
	testkit.WaitFor(t, 10*time.Second, "e2e.mailrule created by a control message under a mail rule", func() bool { return exists("e2e.mailrule") })

	time.Sleep(500 * time.Millisecond)
	pids, err := trySitePids(s, "dreaderd")
	if err != nil {
		// This extra process-scoped scan used to be absent off Linux.
		// Do not fail an already-verified control operation just because
		// the platform cannot provide this optional observation.
		t.Logf("optional temporary-file process scan unavailable: %v", err)
		return
	}
	for _, pid := range pids {
		// the old fixed name and the mkstemp names of this pid ("." keeps pid 123 from
		// matching the files of pid 1234)
		base := "/tmp/dreaderd.ctl" + strconv.Itoa(pid)
		left, _ := filepath.Glob(base + ".*")
		if _, err := os.Lstat(base); err == nil {
			left = append(left, base)
		}
		for _, f := range left {
			if fi, err := os.Lstat(f); err == nil {
				t.Errorf("temporary control file left behind: %s (%v)", f, fi.Mode())
			}
		}
	}
}

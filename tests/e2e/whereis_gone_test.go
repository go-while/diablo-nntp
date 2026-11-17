package e2e

import (
	"strconv"
	"strings"
	"testing"
	"time"

	testkit "github.com/go-while/nntp-testkit"
	"github.com/go-while/nntp-testkit/fakeserver"
)

// TestWhereisArticleGoneBeforeReply: with localspool= dreaderd asks the spool server WHEREIS
// first. A client asks for an article by number, and the article is cancelled before the spool
// server answers 223. The client gets "423", and the connection to the spool server serves the
// next article. Before, dreaderd then waited for the lines of an article it had not asked for:
// the client got no reply, and the connection to the spool server stayed busy.
func TestWhereisArticleGoneBeforeReply(t *testing.T) {
	skipWithoutCanLock(t)
	const hold = 8 * time.Second // the spool server's answer to WHEREIS comes this late
	id, id2 := msgID(t), msgID(t)
	st := fakeserver.NewStore()
	st.AddGroup("e2e.test", "e2e test group")
	srv := fakeserver.New(t, fakeserver.Options{Store: st, Faults: fakeserver.Faults{
		Delay: map[string]time.Duration{"WHEREIS": hold},
		OverrideLine: map[string][]string{
			"WHEREIS " + id + " REL":  {"223 0 " + id + " in D.00000000/B.0000 offset 0 length 10"},
			"WHEREIS " + id2 + " REL": {"430 not here"},
		}}})
	hosts := "127.0.0.1\ts1p" + strconv.Itoa(srv.Port()) + "\tlocalspool=" + t.TempDir() + "\n"
	// readercanlock all and the dcontrol.ctl line: the cancel below is executed
	s := NewReaderSite(t, ReaderOptions{Spool: srv,
		Config: map[string]string{"readercanlock": "all"},
		Files:  map[string]string{"dserver.hosts": hosts, "dcontrol.ctl": "cancel:*:*:doit,break\n"}})
	t.Cleanup(func() {
		if t.Failed() {
			t.Logf("general.log:\n%s", generalLog(s))
			t.Logf("backend got: %q", srv.Lines())
		}
	})
	feedReader(t, s, st, "e2e.test",
		newArticle(id, "e2e.test", "whereis gone", "body"),
		newArticle(id2, "e2e.test", "whereis gone 2", "body"))

	c := s.Reader(t)
	defer c.Close()
	testkit.WaitFor(t, testkit.Deadline(), "the articles in GROUP e2e.test", func() bool {
		_, line := c.Cmd("GROUP e2e.test")
		f := strings.Fields(line)
		return len(f) > 1 && f[0] == "211" && f[1] == "2"
	})
	c.MustSend("ARTICLE 1\r\n")
	testkit.WaitFor(t, 5*time.Second, "WHEREIS at the spool server", func() bool {
		for _, l := range srv.Lines() {
			if strings.HasPrefix(strings.ToUpper(l), "WHEREIS "+strings.ToUpper(id)) {
				return true
			}
		}
		return false
	})
	began := time.Now()

	// the article is cancelled while the spool server has not answered yet
	feedReader(t, s, st, "e2e.test", cancelArticle(t, id, "e2e.test"))
	d := s.Reader(t)
	defer d.Close()
	testkit.WaitFor(t, 4*time.Second, "article 1 gone after the cancel (STAT 1 = 423)", func() bool {
		d.ExpectCmd(211, "GROUP e2e.test")
		code, _ := d.Cmd("STAT 1")
		return code == 423
	})
	if w := time.Since(began); w >= hold-time.Second {
		t.Fatalf("the cancel took %v, the spool server may have answered WHEREIS before it", w)
	}

	c.Deadline = hold + 10*time.Second
	code, line, err := c.ReadStatus()
	if err != nil || code != 423 {
		t.Fatalf("ARTICLE 1, cancelled before the WHEREIS answer: %d %q %v, want 423", code, line, err)
	}
	// the stream to the spool server is in step: the next article comes over it
	c.Deadline = hold + 20*time.Second
	c.ExpectCmd(220, "ARTICLE 2")
	if got := strings.Join(c.ReadDotLines(), "\n"); !strings.Contains(got, "whereis gone 2") {
		t.Errorf("ARTICLE 2: %q, want the second article", got)
	}
	if n := srv.Accepted(); n != 1 {
		t.Errorf("the spool server had %d connections, want 1: dreaderd closed the one in use", n)
	}
}

package e2e

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	testkit "github.com/go-while/nntp-testkit"
	"github.com/go-while/nntp-testkit/fakeserver"
)

// TestWhereisClientGoneBeforeReply: with localspool= dreaderd asks the spool server WHEREIS and
// reads the article from the local file the 223 answer names. The client that asked is gone
// when the answer comes: its idle timeout ended it while it waited. The reader process must
// go on: before, NewDFA() used the client of the request without a check, and the process
// died with all its other clients. (In our run a client that only closed its connection did
// not trigger this.)
func TestWhereisClientGoneBeforeReply(t *testing.T) {
	// The spool server's answer to WHEREIS comes this late. dreaderd looks at a client that
	// waits for a server in its check of all connections, about every 30 s (seen: 42 s):
	// that is when it sees that the client is past its idle timeout, and ends it.
	// The fake's Faults.Delay is cut at the test deadline (30 s), so the handler sleeps itself.
	const hold = 75 * time.Second
	readerPid := regexp.MustCompile(`reader: \S+ \[(\d+)/`)
	for _, tc := range []struct {
		name   string
		access string
	}{
		{"idle-timeout", accessWith("idletimeout 2")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			id, id2 := msgID(t), msgID(t)
			spoolDir := t.TempDir()
			art := "Path: e2e.peer!not-for-mail\nFrom: E2E Test <e2e@example.invalid>\nNewsgroups: e2e.test\n" +
				"Subject: whereis no client\nMessage-ID: " + id + "\n\nbody\n"
			if err := os.WriteFile(filepath.Join(spoolDir, "art1"), []byte(art), 0o644); err != nil {
				t.Fatal(err)
			}
			st := fakeserver.NewStore()
			st.AddGroup("e2e.test", "e2e test group")
			reply223 := "223 0 " + id + " in art1 offset 0 length " + strconv.Itoa(len(art))
			srv := fakeserver.New(t, fakeserver.Options{Handler: fakeserver.LineHandler(
				func(c *fakeserver.Conn, line string) ([]string, bool) {
					f := strings.Fields(line)
					if len(f) > 1 && strings.EqualFold(f[0], "WHEREIS") && f[1] == id {
						time.Sleep(hold)
						return []string{reply223}, false
					}
					if len(f) > 0 && strings.EqualFold(f[0], "QUIT") {
						return []string{"205 bye"}, true
					}
					return []string{"500 What?"}, false
				})})
			hosts := "127.0.0.1\ts1p" + strconv.Itoa(srv.Port()) + "\tlocalspool=" + spoolDir + "\n"
			s := NewReaderSite(t, ReaderOptions{Spool: srv, Access: tc.access,
				Files: map[string]string{"dserver.hosts": hosts}})
			t.Cleanup(func() {
				if t.Failed() {
					t.Logf("general.log:\n%s", generalLog(s))
					t.Logf("backend got: %q", srv.Lines())
				}
			})
			feedReader(t, s, st, "e2e.test",
				newArticle(id, "e2e.test", "whereis no client", "body"),
				newArticle(id2, "e2e.test", "whereis no client 2", "body"))

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
			// the spool server answers 223 after hold; give dreaderd time to act on it
			time.Sleep(hold + 2*time.Second)

			// a client after it is served by the same reader process
			d := s.Reader(t)
			defer d.Close()
			d.ExpectCmd(211, "GROUP e2e.test")
			d.ExpectCmd(223, "STAT 2")
			pids := map[string]bool{}
			for _, m := range readerPid.FindAllStringSubmatch(generalLog(s), -1) {
				pids[m[1]] = true
			}
			if len(pids) != 1 {
				t.Errorf("the log has %d reader processes, want 1: the reader died and was started again", len(pids))
			}
		})
	}
}

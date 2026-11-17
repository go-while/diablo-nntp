package e2e

import (
	"errors"
	"fmt"
	"net"
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
// died with all its other clients. (A client that only closes its connection is not noticed
// while its request is out, and gets the article written into the closed connection.)
func TestWhereisClientGoneBeforeReply(t *testing.T) {
	// The spool server's answer to WHEREIS comes this late. dreaderd looks at a client that
	// waits for a server only in its check of all connections, about every 30 s (seen: 42 s):
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
			asked := time.Now()
			// dreaderd looks at the clock only every 10 selects (2 s each when idle), so a
			// second client keeps its loop turning for the check of all connections
			defer whereisBusy(t, s)()

			// the idle timeout must end the client before the 223 comes, else the test
			// shows nothing: the "timeout idle" line within hold-10 s of the WHEREIS, and
			// the client sees its 400 or EOF/a reset (not the article, not a deadline)
			testkit.WaitFor(t, hold-10*time.Second, "the idle timeout to end the waiting client", func() bool {
				return strings.Contains(generalLog(s), "timeout idle")
			})
			c.Deadline = 10 * time.Second
			for {
				line, err := c.ReadLine()
				if err != nil {
					var ne net.Error
					if errors.As(err, &ne) && ne.Timeout() {
						t.Fatalf("the waiting client was not closed after its idle timeout: %v", err)
					}
					break
				}
				if !strings.HasPrefix(line, "400 ") {
					t.Fatalf("the waiting client got %q, want only the idle timeout's 400 and EOF", line)
				}
			}
			if time.Since(asked) >= hold {
				t.Fatalf("the client was ended only %v after the WHEREIS, not before the 223 at %v", time.Since(asked), hold)
			}

			// the spool server answers 223 after hold; give dreaderd time to act on it
			time.Sleep(time.Until(asked.Add(hold + 2*time.Second)))

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

// whereisBusy keeps dreaderd's loop turning with a client that sends GROUP every 200 ms
// (dreaderd looks at the clock only every 10 selects, 2 s each when idle, so its check of all
// connections, due every 30 s, can come late on an idle process). The returned function stops
// it and fails t if a GROUP of it failed meanwhile.
func whereisBusy(t *testing.T, s *Site) func() {
	t.Helper()
	busy := s.Reader(t)
	stop := make(chan struct{})
	done := make(chan struct{})
	var berr error
	go func() {
		defer close(done)
		for {
			select {
			case <-stop:
				return
			case <-time.After(200 * time.Millisecond):
			}
			if code, line, err := busy.TryCmd("GROUP e2e.test"); err != nil || code != 211 {
				berr = fmt.Errorf("GROUP on the busy client: %d %q (%v)", code, line, err)
				return
			}
		}
	}()
	return func() {
		close(stop)
		<-done
		busy.Close()
		if berr != nil {
			t.Errorf("%v", berr)
		}
	}
}

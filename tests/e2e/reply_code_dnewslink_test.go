package e2e

// Reply codes in dnewslink: the whole three-digit code, compared with the codes the command
// defines; an unknown code is an error, and a reply to MODE STREAM or MODE COMPRESS that is out
// of step does not leave the connection in use.

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/go-while/nntp-testkit/fakeserver"
)

// rcuDnewslink writes the queue file out.S00000 with one entry for id (the spool file of
// dnlSpool), runs dnewslink once on it against srv with the article log (-A) and returns its
// output.
func rcuDnewslink(t *testing.T, s *Site, srv *fakeserver.Server, id string, args ...string) string {
	t.Helper()
	q := "D.00000001/B.0001 " + id + " 0,100\n"
	if err := os.WriteFile(filepath.Join(s.Dir, "dqueue", "out.S00000"), []byte(q), 0o644); err != nil {
		t.Fatal(err)
	}
	a := append([]string{"-Aall", "-N", "1", "-S", "0", "-b", "out.S%05d", "-h", "127.0.0.1",
		"-P", strconv.Itoa(srv.Port()), "-t", "5", "-w", "1"}, args...)
	out, err := s.TryRun("dnewslink", a...)
	t.Logf("dnewslink: err=%v\n%s", err, out)
	return out
}

// rcuQueued fails the test unless the queue file out.S00000 holds id on exactly one line.
func rcuQueued(t *testing.T, s *Site, id string) {
	t.Helper()
	got := dnlQueue(t, s, "out.S00000")
	n := 0
	for _, l := range strings.Split(got, "\n") {
		if strings.Contains(l, id) {
			n++
		}
	}
	if n != 1 {
		t.Errorf("%s is on %d lines of the queue file, want 1 (the article must stay queued); queue file:\n%s", id, n, got)
	}
}

// rcuFeedlog returns dnewslink's article log for the peer 127.0.0.1 ("" when there is none).
func rcuFeedlog(s *Site) string {
	b, _ := os.ReadFile(filepath.Join(s.Dir, "log", "feedlog.127.0.0.1"))
	return string(b)
}

// TestDnewslinkUndefinedTransferCodes: 437 is no answer to an IHAVE offer (only to the article
// after it, RFC 3977 6.3.2), nor to TAKETHIS, and 431 is no answer to TAKETHIS (TAKETHIS has 239
// and 439, RFC 4644). Such a reply fails the connection and the article stays queued; it is not
// logged as rejected. Fails on main: each of them counted as a reject and the article was
// dropped from the queue.
func TestDnewslinkUndefinedTransferCodes(t *testing.T) {
	const id = "<rcu-xfer@e2e.test>"
	for _, tc := range []struct {
		name   string
		stream bool   // the peer streams (CHECK 238 first)
		reply  string // the peer's answer to IHAVE or TAKETHIS
	}{
		{"IHAVE 437", false, "437 " + id + " no"},
		{"TAKETHIS 437", true, "437 " + id + " no"},
		{"TAKETHIS 431", true, "431 " + id + " later"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := NewSite(t, SiteOptions{NoStart: true})
			dnlSpool(t, s)
			srv := fakeserver.New(t, fakeserver.Options{Greeting: "200 fake", Handler: fakeserver.LineHandler(func(_ *fakeserver.Conn, line string) ([]string, bool) {
				l := strings.ToLower(strings.TrimSpace(line))
				switch {
				case strings.HasPrefix(l, "mode stream"):
					if tc.stream {
						return []string{"203 stream ok"}, false
					}
					return []string{"501 no streaming"}, false
				case strings.HasPrefix(l, "check "):
					return []string{"238 " + id}, false
				case strings.HasPrefix(l, "ihave "), strings.HasPrefix(l, "takethis "):
					return []string{tc.reply}, false
				case strings.HasPrefix(l, "quit"):
					return []string{"205 bye"}, true
				}
				return nil, false
			})})
			rcuDnewslink(t, s, srv, id)
			if srv.Accepted() < 1 {
				t.Fatalf("dnewslink did not connect")
			}
			rcuQueued(t, s, id)
			if log := rcuFeedlog(s); strings.Contains(log, id+" rejected") {
				t.Errorf("%q to %s was logged as a reject:\n%s", tc.reply, strings.Fields(tc.name)[0], log)
			}
		})
	}
}

// TestDnewslinkModeReplyOutOfStep: a reply to MODE STREAM or MODE COMPRESS that the command does
// not have (here a code with lines after it) ends the connection; dnewslink does not go on
// without the mode on the same stream, where the left-over line would be read as the answer to
// the first offer. The article stays queued and is not offered. A one-line refusal still
// falls back (TestDnewslinkIHAVEFallback). Fails on main: dnewslink fell back on the same
// connection and took the left-over "435"/"438" line for the peer's refusal of the article,
// which was dropped from the queue without being offered.
func TestDnewslinkModeReplyOutOfStep(t *testing.T) {
	const id = "<rcu-mode@e2e.test>"
	t.Run("MODE STREAM", func(t *testing.T) {
		s := NewSite(t, SiteOptions{NoStart: true})
		dnlSpool(t, s)
		srv := fakeserver.New(t, fakeserver.Options{Greeting: "200 fake", Handler: fakeserver.LineHandler(func(_ *fakeserver.Conn, line string) ([]string, bool) {
			l := strings.ToLower(strings.TrimSpace(line))
			switch {
			case strings.HasPrefix(l, "mode stream"):
				return []string{"215 list follows", "435 " + id + " left over", "."}, false
			case strings.HasPrefix(l, "quit"):
				return []string{"205 bye"}, true
			}
			return nil, false // no answer to IHAVE: only the left-over line could answer it
		})})
		out := rcuDnewslink(t, s, srv, id)
		if srv.Accepted() < 1 {
			t.Fatalf("dnewslink did not connect")
		}
		if n := srv.Count("IHAVE"); n != 0 {
			t.Errorf("IHAVE sent %d times after an out-of-step MODE STREAM reply, want 0 (lines %q)\n%s", n, srv.Lines(), out)
		}
		rcuQueued(t, s, id)
		if log := rcuFeedlog(s); strings.Contains(log, id+" refused") {
			t.Errorf("the left-over line was taken as the refusal of the article:\n%s", log)
		}
	})
	t.Run("MODE COMPRESS", func(t *testing.T) {
		skipWithoutZlib(t)
		s := NewSite(t, SiteOptions{NoStart: true})
		dnlSpool(t, s)
		srv := fakeserver.New(t, fakeserver.Options{Greeting: "200 fake", Handler: fakeserver.LineHandler(func(_ *fakeserver.Conn, line string) ([]string, bool) {
			l := strings.ToLower(strings.TrimSpace(line))
			switch {
			case strings.HasPrefix(l, "mode stream"):
				return []string{"203 stream ok"}, false
			case strings.HasPrefix(l, "mode compress"):
				return []string{"290 odd", "438 " + id + " left over"}, false
			case strings.HasPrefix(l, "quit"):
				return []string{"205 bye"}, true
			}
			return nil, false // no answer to CHECK
		})})
		out := rcuDnewslink(t, s, srv, id, "-Z", "6")
		if srv.Accepted() < 1 {
			t.Fatalf("dnewslink did not connect")
		}
		if n := srv.Count("CHECK"); n != 0 {
			t.Errorf("CHECK sent %d times after an out-of-step MODE COMPRESS reply, want 0 (lines %q)\n%s", n, srv.Lines(), out)
		}
		rcuQueued(t, s, id)
		if log := rcuFeedlog(s); strings.Contains(log, id+" refused") {
			t.Errorf("the left-over line was taken as the refusal of the article:\n%s", log)
		}
	})
}

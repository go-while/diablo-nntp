package e2e

import (
	"strings"
	"testing"
	"time"
)

// postToFeederSite returns a reader site whose outgoing (post) server is a real diablo:
// dreaderd offers each POST there with IHAVE from 127.0.0.1, which the label "reader" lets
// in.
func postToFeederSite(t *testing.T) *Site {
	t.Helper()
	feeds := DefaultNewsfeeds + "label reader\n    inhost 127.0.0.1\n    addgroup *\nend\n"
	feeder := NewSite(t, SiteOptions{Newsfeeds: feeds})
	_, spool := spoolStore(t)
	port := strings.TrimPrefix(feeder.Addr, "127.0.0.1:")
	hosts := "127.0.0.1\ts1p" + strings.TrimPrefix(spool.Addr(), "127.0.0.1:") + "\n" +
		"127.0.0.1\to1p" + port + "\n"
	return NewReaderSite(t, ReaderOptions{Spool: spool, Files: map[string]string{"dserver.hosts": hosts}})
}

// postReply POSTs raw (the final dot line is added) on a new connection and waits up to d
// for the reply; code 0 means no reply came.
func postReply(t *testing.T, s *Site, raw string, d time.Duration) (int, string) {
	t.Helper()
	c := s.Reader(t)
	defer c.Close()
	c.ExpectCmd(340, "POST")
	c.MustSend(raw + ".\r\n")
	c.Deadline = d
	code, line, err := c.ReadStatus()
	if err != nil {
		t.Logf("POST reply: %v", err)
		return 0, line
	}
	return code, line
}

// TestPostBareLFLastLineToFeeder (deep review r2 H1): a POST whose last body line ends in a
// bare LF reaches the post server with that line as it is, followed by ".\r\n". diablo ends
// an article only on CRLF "." CRLF, so it takes the "." as data and waits for more: the post
// connection never gets an answer, the client never gets its reply, and the next POST of the
// reader process queues behind it. TestPostBareLFAccepted does not see it: its fake post
// server ends the article on any ".".
func TestPostBareLFLastLineToFeeder(t *testing.T) {
	s := postToFeederSite(t)
	head := func(id string) string {
		return "From: a <a@example.invalid>\r\nNewsgroups: e2e.test\r\nSubject: to a feeder\r\nMessage-ID: " + id + "\r\n\r\n"
	}

	// control: CRLF line ends throughout, diablo takes the post
	if code, line := postReply(t, s, head(msgID(t))+"first body line\r\nlast body line\r\n", 15*time.Second); code != 240 {
		t.Fatalf("CRLF post to the feeder: %d %q, want 240", code, line)
	}

	code, line := postReply(t, s, head(msgID(t))+"first body line\r\nlast body line\n", 15*time.Second)
	if code != 240 {
		t.Errorf("post whose last body line ends in a bare LF: %d %q, want 240 within 15 s", code, line)
	}
	// the post connection must still work for the next client
	if code, line := postReply(t, s, head(msgID(t))+"after\r\n", 15*time.Second); code != 240 {
		t.Errorf("CRLF post after it: %d %q, want 240 within 15 s", code, line)
	}
}

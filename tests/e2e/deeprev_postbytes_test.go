package e2e

import (
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	testkit "github.com/go-while/nntp-testkit"
)

// TestDeepRevPostBytesOver2G (L4-11): the reader counts the bytes of the articles a session
// sends (IHAVE, TAKETHIS, POST) and reports them to the main process when the session ends:
// "postbytes=" in the "closed from" log line and the FEED/POST byte totals of "drcmd stats".
// The counter was an int, so a feed session that sent more than 2 GiB (hours on a header
// feed, less on a busy one) wrapped it: a negative postbytes= and a total that went down.
//
// One feed session sends 265 articles of about 8 MiB (2.07 GiB); the logged postbytes and
// the FEED total must be at least what was sent.
//
// Takes a minute or two.
func TestDeepRevPostBytesOver2G(t *testing.T) {
	_, spool := spoolStore(t)
	s := NewReaderSite(t, ReaderOptions{Spool: spool})

	const (
		arts     = 265
		bodyLine = 998
		lines    = 8400 // 8400 * 1000 bytes: about 8 MiB per article
	)
	body := strings.Repeat(strings.Repeat("x", bodyLine)+"\r\n", lines) + ".\r\n"
	sent := int64(0)

	p := s.dialFrom(t, PeerIP)
	p.Deadline = 60 * time.Second
	for range arts {
		id := msgID(t)
		head := strings.Join(newArticle(id, "e2e.test", "postbytes"), "\r\n") + "\r\n"
		p.ExpectCmd(335, "IHAVE %s", id)
		if err := p.Send(head); err != nil {
			t.Fatalf("send: %v", err)
		}
		if err := p.Send(body); err != nil {
			t.Fatalf("send: %v", err)
		}
		// accepted or not: the bytes are counted as they come in
		if _, line, err := p.ReadStatus(); err != nil {
			t.Fatalf("IHAVE %s: %q: %v", id, line, err)
		}
		sent += int64(len(body) - 3)
	}
	p.ExpectCmd(205, "QUIT")
	if sent <= 1<<31 {
		t.Fatalf("the test sent %d bytes, it must send more than 2 GiB", sent)
	}

	re := regexp.MustCompile(`closed from .*\(` + regexp.QuoteMeta(PeerIP) + `:[0-9]+\).* postbytes=(-?[0-9]+) `)
	var m []string
	testkit.WaitFor(t, testkit.Deadline(), "the \"closed from\" line of the feed session in the general log", func() bool {
		m = re.FindStringSubmatch(generalLog(s))
		return m != nil
	})
	if n, _ := strconv.ParseInt(m[1], 10, 64); n < sent {
		t.Errorf("the session sent %d bytes of articles, the log has postbytes=%s: the counter wrapped at 2 GiB\n%s", sent, m[1], m[0])
	}

	out := s.Run(t, "drcmd", "rawstats")
	fm := regexp.MustCompile(`FEED Articles=[0-9]+ Bytes=(-?[0-9]+)`).FindStringSubmatch(out)
	if fm == nil {
		t.Fatalf("drcmd rawstats has no FEED line:\n%s", out)
	}
	if n, _ := strconv.ParseInt(fm[1], 10, 64); n < sent {
		t.Errorf("the session sent %d bytes of articles, drcmd rawstats has FEED Bytes=%s", sent, fm[1])
	}
}

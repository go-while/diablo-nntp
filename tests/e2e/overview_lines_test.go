package e2e

import (
	"maps"
	"net"
	"regexp"
	"slices"
	"strings"
	"testing"

	testkit "github.com/go-while/nntp-testkit"
	"github.com/go-while/nntp-testkit/client"
	"github.com/go-while/nntp-testkit/fakeserver"
)

// withHeader inserts a header line into an article built by newArticle.
func withHeader(art []string, h string) []string {
	i := slices.Index(art, "")
	return slices.Concat(art[:i], []string{h}, art[i:])
}

// overLines returns the 8th field (:lines) of each OVER line, by article number.
func overLines(t *testing.T, c *client.Conn, cmd string) map[string]string {
	t.Helper()
	c.ExpectCmd(224, "%s", cmd)
	got := map[string]string{}
	for _, l := range c.ReadDotLines() {
		f := strings.Split(l, "\t")
		if len(f) < 8 {
			t.Fatalf("%s: %q has %d fields", cmd, l, len(f))
		}
		got[f[0]] = f[7]
	}
	return got
}

func waitGroupCount(t *testing.T, c *client.Conn, n string) {
	t.Helper()
	testkit.WaitFor(t, testkit.Deadline(), n+" articles in GROUP e2e.test", func() bool {
		_, line := c.Cmd("GROUP e2e.test")
		f := strings.Fields(line)
		return len(f) > 1 && f[0] == "211" && f[1] == n
	})
}

// checkLines checks OVER, XOVER, HDR :lines and XHDR Lines against want (by article number).
func checkLines(t *testing.T, c *client.Conn, want map[string]string) {
	t.Helper()
	for _, cmd := range []string{"OVER 1-", "XOVER 1-"} {
		if got := overLines(t, c, cmd); !maps.Equal(got, want) {
			t.Errorf("%s :lines %v, want %v", cmd, got, want)
		}
	}
	for _, cmd := range []string{"HDR :lines 1-", "XHDR Lines 1-"} {
		code, _ := c.Cmd("%s", cmd)
		if code != 221 && code != 225 {
			t.Errorf("%s: %d", cmd, code)
			continue
		}
		got := map[string]string{}
		for _, l := range c.ReadDotLines() {
			n, v, _ := strings.Cut(l, " ")
			got[n] = v
		}
		if !maps.Equal(got, want) {
			t.Errorf("%s %v, want %v", cmd, got, want)
		}
	}
}

// TestReaderOverviewLinesFullFeed (D-174): on a full feed dreaderd counts the body lines
// itself: the :lines field (RFC 3977 8.4.2) is the count, with or without a Lines header,
// and a wrong Lines header or a poster's Bytes header does not change it. Before, the field
// was the Lines header, empty without one.
func TestReaderOverviewLinesFullFeed(t *testing.T) {
	st, spool := spoolStore(t)
	s := NewReaderSite(t, ReaderOptions{Spool: spool})
	feedReader(t, s, st, "e2e.test",
		newArticle(msgID(t), "e2e.test", "no Lines header", "one", "two", "three"),
		withHeader(newArticle(msgID(t), "e2e.test", "wrong Lines header", "one", "two"), "Lines: 99"),
		withHeader(newArticle(msgID(t), "e2e.test", "poster's Bytes", ".dot", "", "end"), "Bytes: 10 50"),
	)
	c := s.Reader(t)
	waitGroupCount(t, c, "3")
	checkLines(t, c, map[string]string{"1": "3", "2": "2", "3": "3"})
	c.ExpectCmd(205, "QUIT")
}

// headFeedLines feeds three articles to a feeder, sends them with dnewslink -H to a reader
// and returns the reader and the feeder's queue lines. With stripL the queue's L<lines>
// fields are removed first, as in a queue an older diablo wrote.
func headFeedLines(t *testing.T, stripL bool) (*client.Conn, []string) {
	t.Helper()
	const label = "reader"
	feeds := GlobalLabel + `label peer
    inhost ` + PeerIP + `
    alias e2e.peer
    addgroup *
end
label ` + label + `
    inhost 127.0.0.1
    hostname 127.0.0.1
    alias dummy
    headfeed on
    addgroup *
end
`
	feeder := NewSite(t, SiteOptions{Newsfeeds: feeds})
	r := NewReaderSite(t, ReaderOptions{SpoolAddr: feeder.Addr})
	p := feeder.Peer(t)
	for _, a := range [][]string{
		newArticle(msgID(t), "e2e.test", "no Lines header", "one", "two", "three"),
		withHeader(newArticle(msgID(t), "e2e.test", "wrong Lines header", "one", "two"), "Lines: 99"),
		newArticle(msgID(t), "e2e.test", "dot and empty lines", ".dot", "", "end", ""),
	} {
		id := headerValue(a[:slices.Index(a, "")], "Message-ID")
		p.ExpectCmd(335, "IHAVE %s", id)
		p.MustSend(client.Stuff(a))
		p.ExpectStatus(235, "IHAVE "+id)
	}
	p.ExpectCmd(205, "QUIT")
	q := feeder.waitQueue(t, label, 3)
	if stripL {
		var b strings.Builder
		for _, l := range q {
			b.WriteString(regexp.MustCompile(` L[0-9]+`).ReplaceAllString(l, "") + "\n")
		}
		feeder.WriteFile(t, "dqueue/"+label, b.String())
	}

	_, rport, _ := net.SplitHostPort(r.Addr)
	feeder.Run(t, "dnewslink", "-b", label, "-h", "127.0.0.1", "-P", rport, "-H", "-B", PeerIP)

	c := r.Reader(t)
	waitGroupCount(t, c, "3")
	return c, q
}

// TestReaderOverviewLinesHeaderFeed (D-174): diablo counts the body lines when the article
// comes in and writes them to the queue line (L<lines>, the "." line not counted); dnewslink
// -H sends them with the size ("Bytes: <size> <lines>") without reading the bodies, and the
// reader's :lines is that count.
func TestReaderOverviewLinesHeaderFeed(t *testing.T) {
	c, q := headFeedLines(t, false)
	for i, want := range []string{" L3", " L2", " L4"} {
		if !strings.Contains(q[i]+" ", want+" ") {
			t.Errorf("queue line %d %q, want %s", i+1, q[i], want)
		}
	}
	checkLines(t, c, map[string]string{"1": "3", "2": "2", "3": "4"})
	// the size field stays a number, and the Bytes: line is not stored as a header
	c.ExpectCmd(224, "OVER 1-")
	for _, l := range c.ReadDotLines() {
		if f := strings.Split(l, "\t"); len(f) < 7 || strings.Trim(f[6], "0123456789") != "" || f[6] == "" {
			t.Errorf("OVER :bytes field of %q is not a number", l)
		}
	}
	c.ExpectCmd(221, "HEAD 1")
	for _, l := range c.ReadDotLines() {
		if strings.HasPrefix(strings.ToLower(l), "bytes:") {
			t.Errorf("HEAD 1 shows the feed's %q", l)
		}
	}
	c.ExpectCmd(205, "QUIT")
}

// TestReaderOverviewLinesOldQueue (D-174): a queue line without L<lines> (written by an older
// diablo) gives "Bytes: <size>" alone, and the reader keeps the Lines header, as before.
func TestReaderOverviewLinesOldQueue(t *testing.T) {
	c, _ := headFeedLines(t, true)
	if got := overLines(t, c, "OVER 1-"); !maps.Equal(got, map[string]string{"1": "", "2": "99", "3": ""}) {
		t.Errorf("OVER :lines %v, want the Lines headers (empty, 99, empty)", got)
	}
	c.ExpectCmd(205, "QUIT")
}

// TestReaderOverviewLinesOldHeaderFeed (D-174): a header feed from an older dnewslink sends
// "Bytes: <size>" alone; the reader then keeps the Lines header for :lines, as before.
func TestReaderOverviewLinesOldHeaderFeed(t *testing.T) {
	st, spool := spoolStore(t)
	s := NewReaderSite(t, ReaderOptions{Spool: spool})
	p := s.dialFrom(t, PeerIP)
	p.ExpectCmd(250, "MODE HEADFEED")
	arts := [][]string{
		withHeader(newArticle(msgID(t), "e2e.test", "with Lines"), "Lines: 7"),
		newArticle(msgID(t), "e2e.test", "without Lines"),
	}
	for _, a := range arts {
		head, _ := splitArticle(a)
		id := headerValue(head, "Message-ID")
		st.AddArticle("e2e.test", fakeserver.Article{MessageID: id, Headers: head, Body: []string{"x"}})
		p.ExpectCmd(335, "IHAVE %s", id)
		p.MustSend(client.Stuff(withHeader(a, "Bytes: 500")))
		p.ExpectStatus(235, "IHAVE "+id)
	}
	p.ExpectCmd(205, "QUIT")
	c := s.Reader(t)
	waitGroupCount(t, c, "2")
	if got := overLines(t, c, "OVER 1-"); !maps.Equal(got, map[string]string{"1": "7", "2": ""}) {
		t.Errorf("OVER :lines %v, want the Lines header (7) and empty", got)
	}
	c.ExpectCmd(205, "QUIT")
}

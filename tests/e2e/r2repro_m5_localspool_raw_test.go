package e2e

// Deep review round 2, M5 (reproduction): localspool= with a real diablo as the spool server. The
// feeder's whereis answer names the whole spool record (boffset: the start of the 24-byte
// SpoolArtHdr), which dreaderd reads as article text (util/diablo.c whereis, dreaderd/spool.c, dfa.c).

import (
	"strings"
	"testing"

	"github.com/go-while/nntp-testkit/client"
)

// r2ArticleVia fetches the article with ARTICLE <id> from a reader whose only spool server is the
// diablo site f, with or without localspool=dir, and returns the status line and the lines.
func r2ArticleVia(t *testing.T, f *Site, localspool, id string) (string, []string) {
	t.Helper()
	port := f.Addr[strings.LastIndex(f.Addr, ":")+1:]
	hosts := "127.0.0.1\ts1p" + port + "\n"
	if localspool != "" {
		hosts = "127.0.0.1\ts1p" + port + " localspool=" + localspool + "\n"
	}
	s := NewReaderSite(t, ReaderOptions{SpoolAddr: f.Addr, Files: map[string]string{"dserver.hosts": hosts}})
	x := r2Dial(t, s.Addr)
	x.send("ARTICLE " + id + "\r\n")
	st, err := x.line(30e9)
	if err != nil {
		t.Fatalf("ARTICLE %s (localspool %q): %v", id, localspool, err)
	}
	var lines []string
	if strings.HasPrefix(st, "220") {
		lines, err = x.block(30e9)
		if err != nil {
			t.Fatalf("ARTICLE %s (localspool %q): %v after %q", id, localspool, err, lines)
		}
	}
	return st, lines
}

// TestR2ReproLocalspoolDiabloSpool: the same articles through a reader with localspool= on the
// feeder's spool and through one that asks the feeder over NNTP must come out the same.
func TestR2ReproLocalspoolDiabloSpool(t *testing.T) {
	nf := GlobalLabel + "label peer\n    inhost " + PeerIP + "\n    addgroup *\nend\n" +
		"label rdr\n    inhost 127.0.0.1\n    readonly yes\n    whereis yes\n    addgroup *\nend\n"
	f := NewSite(t, SiteOptions{Newsfeeds: nf})
	spoolDir := strings.TrimSpace(f.Run(t, "dpath", "spool"))
	t.Logf("feeder spool directory: %s", spoolDir)

	type art struct {
		name string
		id   string
		text []string
	}
	arts := []art{
		{name: "plain article"},
		// a header line of 5 characters is the last one before the blank line (dfa.c state 5), and a
		// body line "." is sent as ".." and must not end the reply
		{name: "5-character last header and a dot line"},
	}
	arts[0].id, arts[1].id = msgID(t), msgID(t)
	arts[0].text = newArticle(arts[0].id, "e2e.test", "localspool plain", "line one", "line two")
	head, body := splitArticle(newArticle(arts[1].id, "e2e.test", "localspool dfa5", "first", ".", "last"))
	arts[1].text = append(append(append(head, "Xy: z"), ""), body...)

	p := f.dialFrom(t, PeerIP)
	for _, a := range arts {
		p.ExpectCmd(335, "IHAVE %s", a.id)
		p.MustSend(client.Stuff(a.text))
		p.ExpectStatus(235, "IHAVE "+a.id)
	}
	p.ExpectCmd(205, "QUIT")

	for _, a := range arts {
		stW, wire := r2ArticleVia(t, f, "", a.id)
		stL, local := r2ArticleVia(t, f, spoolDir, a.id)
		t.Logf("%s: over the wire %q (%d lines); localspool %q (%d lines)", a.name, stW, len(wire), stL, len(local))
		if stW != stL || strings.Join(wire, "\n") != strings.Join(local, "\n") {
			t.Errorf("%s: localspool differs from the fetch over NNTP\nwire:  %q\nlocal: %q", a.name, wire, local)
		}
	}
}

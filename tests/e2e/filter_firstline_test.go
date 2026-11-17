package e2e

// The external spam filter (feederfilter) gets each article dot-stuffed and ended by ".", one
// answer per article (deep review r2 M13, D-240).

import (
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/go-while/nntp-testkit/client"
)

// filterNewsfeeds: the peer feeds everything; OutLabel ("fake") gets what the external
// filter did not call spam (nospam), "spamonly" only the spam (onlyspam).
const filterNewsfeeds = GlobalLabel + `label peer
    inhost ` + PeerIP + `
    alias e2e.peer
    addgroup *
end
label ` + OutLabel + `
    hostname 127.0.0.1
    alias e2e.fake
    addgroup *
    nospam
end
label spamonly
    hostname 127.0.0.1
    alias e2e.spamonly
    addgroup *
    onlyspam
end
label ESPAM
    addgroup *
end
`

// filterSite starts a site whose feederfilter is the shell script filter (@home@ replaced),
// with the extra diablo.config settings conf.
func filterSite(t *testing.T, filter, spoolCtl string, conf map[string]string) *Site {
	t.Helper()
	config := map[string]string{"feederfilter": "@home@/filter"}
	maps.Copy(config, conf)
	return NewSite(t, SiteOptions{
		Config:    config,
		Newsfeeds: filterNewsfeeds,
		SpoolCtl:  spoolCtl,
		Setup: func(tb testing.TB, s *Site) {
			if err := os.WriteFile(filepath.Join(s.Dir, "filter"), []byte(s.expand(filter)), 0o755); err != nil {
				tb.Fatal(err)
			}
		},
	})
}

// injectLines offers one article (its lines, stuffed here) from the peer; it must be taken.
func injectLines(t *testing.T, s *Site, id string, lines []string) {
	t.Helper()
	p := s.Peer(t)
	defer p.Close()
	p.ExpectCmd(335, "IHAVE %s", id)
	p.MustSend(client.Stuff(lines))
	p.ExpectStatus(235, "IHAVE "+id)
}

// queued reports which of ids are in dqueue/label now.
func queued(t *testing.T, s *Site, label string, ids ...string) []bool {
	t.Helper()
	q := strings.Join(s.queue(t, label), "\n")
	var in []bool
	for _, id := range ids {
		in = append(in, strings.Contains(q, id))
	}
	return in
}

// perDotFilter answers after every "." line it reads: "435 spam" for an article with the
// subject "spam", else 335. It logs every line it gets and pauses after each answer, so two
// answers never reach diablo in one read.
const perDotFilter = `#!/bin/sh
cr=$(printf '\r')
ans=335
while IFS= read -r line; do
    printf '%s\n' "$line" >> @home@/filter.log
    case "$line" in
    "Subject: spam$cr") ans="435 spam" ;;
    ".$cr") echo "$ans"; ans=335; sleep 0.3 ;;
    esac
done
`

// TestFilterFirstLineDotStuffed (deep review r2 M13, D-240): DiabFilter() dot-stuffed a line
// starting with "." only after a line end it had seen, and it started with none, so an article
// whose first line is "." (sent as "..", unstuffed and stored by the feeder) reached the filter
// as ".\r\n": the filter ended that article at once and answered twice for it. diablo reads one
// answer per article, so every later article got the previous one's verdict. With wireformat
// the article is stored stuffed and must reach the filter stuffed once, not twice. A body line
// "." (in "ham 2") is stuffed as before. Fails on main (text): the first line is ".\r" and the
// verdicts are shifted by one from the crafted article on.
func TestFilterFirstLineDotStuffed(t *testing.T) {
	for _, mode := range []string{"text", "wireformat"} {
		t.Run(mode, func(t *testing.T) {
			conf := map[string]string{}
			if mode == "wireformat" {
				conf["wireformat"] = "on"
			}
			s := filterSite(t, perDotFilter, "", conf)
			crafted := msgID(t)
			injectLines(t, s, crafted, append([]string{"."}, newArticle(crafted, "e2e.test", "crafted", "body")...))
			ids := []string{crafted}
			bodies := map[string][]string{"spam": {"body"}, "ham 2": {".", "after a dot line"}, "ham 3": {"body"}}
			for _, subj := range []string{"spam", "ham 2", "ham 3"} {
				id := msgID(t)
				injectLines(t, s, id, newArticle(id, "e2e.test", subj, bodies[subj]...))
				ids = append(ids, id)
			}

			var got []string
			limit := time.Now().Add(15 * time.Second)
			for {
				b, _ := os.ReadFile(filepath.Join(s.Dir, "filter.log"))
				got = strings.Split(string(b), "\n")
				if strings.Contains(string(b), "Subject: ham 3") || time.Now().After(limit) {
					break
				}
				time.Sleep(100 * time.Millisecond)
			}
			if len(got) == 0 || got[0] != "..\r" {
				t.Errorf("first line the filter got: %q, want the stuffed \"..\\r\"", got[:min(len(got), 3)])
			}
			if n := strings.Count(strings.Join(got, "\n"), "\n...\r\n"); n != 0 {
				t.Errorf("filter got %d lines \"...\": a line stuffed twice", n)
			}

			time.Sleep(time.Second) // the last answer comes 0.3 s after the one before it
			ham := queued(t, s, OutLabel, ids...)
			spam := queued(t, s, "spamonly", ids...)
			if want := []bool{true, false, true, true}; !slices.Equal(ham, want) {
				t.Errorf("queued for the nospam feed (crafted, spam, ham 2, ham 3): %v, want %v", ham, want)
			}
			if want := []bool{false, true, false, false}; !slices.Equal(spam, want) {
				t.Errorf("queued for the onlyspam feed (crafted, spam, ham 2, ham 3): %v, want %v", spam, want)
			}
		})
	}
}

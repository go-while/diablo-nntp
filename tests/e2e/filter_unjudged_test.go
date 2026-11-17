package e2e

import (
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

// filterSite starts a site whose feederfilter is the shell script filter (@home@ replaced).
func filterSite(t *testing.T, filter, spoolCtl string) *Site {
	t.Helper()
	return NewSite(t, SiteOptions{
		Config:    map[string]string{"feederfilter": "@home@/filter"},
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

// TestFilterFirstLineDotStuffed (deep review r2 M13): DiabFilter() dot-stuffs a line that
// starts with "." only after a line end it has seen, and it starts with none, so an article
// whose first line is "." (sent as "..", unstuffed and stored by the feeder) reaches the
// filter as ".\r\n": the filter ends that article at once and answers twice for it. diablo
// reads one answer per article, so every later article gets the previous one's verdict.
func TestFilterFirstLineDotStuffed(t *testing.T) {
	s := filterSite(t, perDotFilter, "")
	crafted := msgID(t)
	injectLines(t, s, crafted, append([]string{"."}, newArticle(crafted, "e2e.test", "crafted", "body")...))
	ids := []string{crafted}
	for _, subj := range []string{"spam", "ham 2", "ham 3"} {
		id := msgID(t)
		injectLines(t, s, id, newArticle(id, "e2e.test", subj, "body"))
		ids = append(ids, id)
	}

	var got []string
	limit := time.Now().Add(15 * time.Second)
	for {
		b, _ := os.ReadFile(filepath.Join(s.Dir, "filter.log"))
		got = strings.Split(string(b), "\n")
		if strings.Count(string(b), "Subject: ham 3") > 0 || time.Now().After(limit) {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if len(got) == 0 || got[0] != "..\r" {
		t.Errorf("first line the filter got: %q, want the stuffed \"..\\r\"", got[:min(len(got), 3)])
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
}

// allSpamFilter calls every article spam.
const allSpamFilter = `#!/bin/sh
cr=$(printf '\r')
while IFS= read -r line; do
    [ "$line" = ".$cr" ] && echo "435 spam"
done
`

// TestFilterUnreadArticlesPass (deep review r2 M14): diab_read() fails on any NUL byte in
// the stored record and on every record of a compressed spool object; DiabFilter() then
// returns -1 without sending anything, and -1 counts as "not spam". With a filter that
// calls everything spam, such an article must not reach the nospam feed.
func TestFilterUnreadArticlesPass(t *testing.T) {
	check := func(t *testing.T, s *Site, control, odd string) {
		t.Helper()
		ids := []string{control, odd}
		var spam []bool
		for limit := time.Now().Add(10 * time.Second); time.Now().Before(limit); time.Sleep(200 * time.Millisecond) {
			if spam = queued(t, s, "spamonly", ids...); spam[0] && spam[1] {
				break
			}
		}
		ham := queued(t, s, OutLabel, ids...)
		if ham[1] || !spam[1] {
			t.Errorf("article the filter calls spam: nospam feed %v, onlyspam feed %v, want false, true", ham[1], spam[1])
		}
		if ham[0] || !spam[0] {
			t.Errorf("control article: nospam feed %v, onlyspam feed %v, want false, true", ham[0], spam[0])
		}
	}

	t.Run("nul", func(t *testing.T) {
		s := filterSite(t, allSpamFilter, "")
		a, b := msgID(t), msgID(t)
		injectLines(t, s, a, newArticle(a, "e2e.test", "plain", "body"))
		injectLines(t, s, b, newArticle(b, "e2e.test", "with a NUL", "a\x00b"))
		check(t, s, a, b)
	})

	t.Run("compressed", func(t *testing.T) {
		skipWithoutZlib(t)
		plain := filterSite(t, allSpamFilter, "")
		a := msgID(t)
		injectLines(t, plain, a, newArticle(a, "e2e.test", "plain spool", "body"))
		s := filterSite(t, allSpamFilter, compressedSpoolCtl)
		b := msgID(t)
		injectLines(t, s, b, newArticle(b, "e2e.test", "compressed spool", "body"))
		var spam []bool
		for limit := time.Now().Add(10 * time.Second); time.Now().Before(limit); time.Sleep(200 * time.Millisecond) {
			if spam = queued(t, plain, "spamonly", a); spam[0] {
				break
			}
		}
		if !spam[0] || queued(t, plain, OutLabel, a)[0] {
			t.Fatalf("control on a plain spool: not judged spam")
		}
		for limit := time.Now().Add(10 * time.Second); time.Now().Before(limit); time.Sleep(200 * time.Millisecond) {
			if spam = queued(t, s, "spamonly", b); spam[0] {
				break
			}
		}
		if ham := queued(t, s, OutLabel, b); ham[0] || !spam[0] {
			t.Errorf("article on a compressed spool: nospam feed %v, onlyspam feed %v, want false, true", ham[0], spam[0])
		}
	})
}

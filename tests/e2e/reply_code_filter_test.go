package e2e

// Reply codes from the external spam filter (feederfilter): the whole three-digit code. cleanfeed
// in highwind mode, the filter usually used with Diablo, answers exactly "335" or "435".

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// rcuFilter is an external filter (feederfilter): it answers each article after its "." line,
// 335 (take it) in general, "435 spam" for the article with the subject "injected 2" and
// "400 filter broken" for the one with "injected 3".
const rcuFilter = `#!/bin/sh
cr=$(printf '\r')
ans=335
while IFS= read -r line; do
    case "$line" in
    "Subject: injected 2$cr") ans="435 spam" ;;
    "Subject: injected 3$cr") ans="400 filter broken" ;;
    ".$cr") echo "$ans"; ans=335 ;;
    esac
done
`

// TestDiabFilterAnswerCodes: the external spam filter's answer is a whole code: 335 takes the
// article, 435 (and 437) mark it as spam, and any other answer is a failed filter, which marks
// nothing. The "nospam" feed gets the articles the filter took (335) and the one it failed on
// ("400 filter broken"), not the spam (435); the "onlyspam" feed gets only the spam. Fails on main: every answer that
// started with 4 was spam, so the "400" article was not queued.
func TestDiabFilterAnswerCodes(t *testing.T) {
	newsfeeds := GlobalLabel + `label peer
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
	s := NewSite(t, SiteOptions{
		Config:    map[string]string{"feederfilter": "@home@/rcufilter"},
		Newsfeeds: newsfeeds,
		Setup: func(tb testing.TB, s *Site) {
			if err := os.WriteFile(filepath.Join(s.Dir, "rcufilter"), []byte(rcuFilter), 0o755); err != nil {
				tb.Fatal(err)
			}
		},
	})
	ids := s.Inject(t, "e2e.test", 3)
	q := strings.Join(s.waitQueue(t, OutLabel, 2), "\n")
	for i, want := range []bool{true, false, true} {
		if got := strings.Contains(q, ids[i]); got != want {
			t.Errorf("article %d (%s) queued for the nospam feed: %v, want %v; queue:\n%s", i+1, ids[i], got, want, q)
		}
	}
	// the unknown answer restarts the filter (filter_failed), so the next answer is not read
	// out of step
	if log := s.logOutput(); !strings.Contains(log, "filter failed, unknown response") {
		t.Errorf("no filter restart logged after the \"400\" answer; log:\n%s", log)
	}
	sq := strings.Join(s.waitQueue(t, "spamonly", 1), "\n")
	for i, want := range []bool{false, true, false} {
		if got := strings.Contains(sq, ids[i]); got != want {
			t.Errorf("article %d (%s) queued for the onlyspam feed: %v, want %v; queue:\n%s", i+1, ids[i], got, want, sq)
		}
	}
}

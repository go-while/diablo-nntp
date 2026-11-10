package e2e

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"syscall"
	"testing"
	"time"
)

// TestDeepRevDspooloutHoursSecondInterval (L3-03): the dnewsfeeds option "hours" takes a
// comma list of hours and ranges ("20,3-7"). dspoolout walks the labels twice (pass 1 rotates
// the queue files, pass 2 starts dnewslink) and split the list with strtok() on the stored
// string itself, which cuts it at the first comma: pass 2 only saw the first entry. When the
// current hour was in a later entry, pass 1 rotated the queue and pass 2 skipped the label, so
// no dnewslink was started for it (and the flags of every later label were taken from the
// wrong slot).
//
// A label whose "hours" names another hour first and the current hours second: dspoolout must
// start a dnewslink for its queue, which connects to the peer. With the current hours first it
// always did (checked so the test's set-up is known to work).
func TestDeepRevDspooloutHoursSecondInterval(t *testing.T) {
	for _, c := range []struct {
		name         string
		currentFirst bool
	}{
		{"second", false},
		{"first", true},
	} {
		t.Run(c.name, func(t *testing.T) {
			peer := newHoldPeer(t)
			// The current hour and the next (the run may cross the full hour), and one that
			// is neither. No leading zeros: dspoolout reads the numbers with base 0.
			h := time.Now().Hour()
			current := fmt.Sprintf("%d-%d", h, (h+1)%24)
			other := strconv.Itoa((h + 12) % 24)
			hours := other + "," + current
			if c.currentFirst {
				hours = current + "," + other
			}
			feeds := GlobalLabel + `label ` + OutLabel + `
    hostname 127.0.0.1
    port ` + strconv.Itoa(peer.Port()) + `
    alias e2e.fake
    addgroup *
    hours ` + hours + `
end
`
			s := NewSite(t, SiteOptions{Newsfeeds: feeds, Setup: linkBin})
			t.Cleanup(func() {
				// The peer never answers, so the dnewslink stays: kill what dspoolout started.
				for range 20 {
					var pids []int
					for _, name := range []string{"dnewslink", "dspoolout"} {
						p, err := trySitePids(s, name)
						if err != nil {
							t.Logf("cleanup: cannot list the %s processes of the site: %v", name, err)
							return
						}
						pids = append(pids, p...)
					}
					if len(pids) == 0 {
						return
					}
					for _, pid := range pids {
						_ = syscall.Kill(pid, syscall.SIGKILL)
					}
					time.Sleep(250 * time.Millisecond)
				}
			})

			q := filepath.Join(s.Dir, "dqueue")
			if err := os.WriteFile(filepath.Join(q, OutLabel),
				[]byte("D.00000000/B.0000 0,100 <hours@e2e.test>\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			out := s.Run(t, "dspoolout")

			limit := time.Now().Add(15 * time.Second)
			for peer.Accepted() == 0 && time.Now().Before(limit) {
				time.Sleep(50 * time.Millisecond)
			}
			if peer.Accepted() == 0 {
				names, _ := filepath.Glob(filepath.Join(q, "*"))
				t.Errorf("hours %s at hour %d: dspoolout started no dnewslink for the label (no connection to the peer within 15 s)\n"+
					"dspoolout:\n%s\ndqueue: %q", hours, h, out, names)
			}
		})
	}
}

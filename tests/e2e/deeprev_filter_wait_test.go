package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// deepRevSlowFilter is an external filter (feederfilter) that passes every article and,
// when diablo closes its input, takes two seconds to end, with exit status 3.
const deepRevSlowFilter = `#!/bin/sh
cr=$(printf '\r')
while IFS= read -r line; do
    [ "$line" = ".$cr" ] && echo 335
done
sleep 2
exit 3
`

// TestDeepRevFilterCloseWaits (L2-07): DiabFilterClose() waits for the external filter with
// waitpid(WNOHANG) once a second, up to ten times. While the filter is still running
// waitpid() returns 0 and does not set the status, and the code tested WIFEXITED() on it
// all the same: what the stack held decided whether diablo logged "exited normally" for a
// filter that was still there, forgot its pid and did not wait at all. It must wait and log
// how the filter really ended.
func TestDeepRevFilterCloseWaits(t *testing.T) {
	s := NewSite(t, SiteOptions{
		Config:    map[string]string{"feederfilter": "@home@/slowfilter"},
		Newsfeeds: DefaultNewsfeeds + "label ESPAM\n    addgroup *\nend\n",
		Setup: func(tb testing.TB, s *Site) {
			if err := os.WriteFile(filepath.Join(s.Dir, "slowfilter"), []byte(deepRevSlowFilter), 0o755); err != nil {
				tb.Fatal(err)
			}
		},
	})
	s.Inject(t, "e2e.test", 1)
	limit := time.Now().Add(10 * time.Second)
	for !strings.Contains(generalLog(s), "External filter launched") {
		if time.Now().After(limit) {
			t.Fatalf("diablo did not start the external filter:\n%s", generalLog(s))
		}
		time.Sleep(100 * time.Millisecond)
	}

	s.Dicmd(t, "exit")
	select {
	case <-s.Proc.Done():
	case <-time.After(20 * time.Second):
		t.Fatalf("diablo still running 20s after dicmd exit")
	}
	var filter []string
	for _, l := range strings.Split(generalLog(s), "\n") {
		if strings.Contains(l, "ilter") {
			filter = append(filter, l)
		}
	}
	if !strings.Contains(strings.Join(filter, "\n"), "External filter returned exit 3") {
		t.Errorf("diablo did not wait for the external filter (it ends 2s after its input is closed, with exit 3); the log has:\n%s",
			strings.Join(filter, "\n"))
	}
}

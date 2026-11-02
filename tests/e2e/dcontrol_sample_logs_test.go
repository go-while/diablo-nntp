package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestDcontrolSampleCheckgroupsLog uses each shipped configuration, rather than
// a reduced copy, so an overlapping later rule cannot silently redirect the log.
func TestDcontrolSampleCheckgroupsLog(t *testing.T) {
	for _, sample := range []string{
		"samples/dcontrol.ctl",
		"samples/reader/dcontrol.ctl",
		"samples/feeder+reader/dcontrol.ctl",
	} {
		t.Run(sample, func(t *testing.T) {
			data, err := os.ReadFile(filepath.Join(repoRoot(t), sample))
			if err != nil {
				t.Fatal(err)
			}
			st, spool := spoolStore(t)
			s := NewReaderSite(t, ReaderOptions{
				Spool: spool,
				Files: map[string]string{"dcontrol.ctl": string(data)},
			})
			var ids []string
			for _, from := range []string{"e2e@example.invalid", "moderator@dana.de"} {
				id := msgID(t)
				ids = append(ids, id)
				a := newArticle(id, "e2e.test", "sample checkgroups log", "e2e.test\tA test group.")
				a[1] = "From: E2E Test <" + from + ">"
				a = append(a[:3:3], append([]string{"Control: checkgroups"}, a[3:]...)...)
				// The second sender has a later matching verify rule without a log=.
				// No signature is supplied: the test checks logging, not execution.
				feedReader(t, s, st, "e2e.test", a)
			}

			want := filepath.Join(s.Dir, "log", "checkgroups.other")
			other := filepath.Join(s.Dir, "log", "control.log")
			waitFor(t, 10*time.Second, "control message to be logged", func() bool {
				correct, _ := os.ReadFile(want)
				fallback, _ := os.ReadFile(other)
				for _, id := range ids {
					if !strings.Contains(string(correct), id) && !strings.Contains(string(fallback), id) {
						return false
					}
				}
				return true
			})
			log, err := os.ReadFile(want)
			if err != nil {
				t.Fatal(err)
			}
			for _, id := range ids {
				if !strings.Contains(string(log), id) {
					t.Errorf("control %s was not written to checkgroups.other", id)
				}
			}
			log, err = os.ReadFile(other)
			if err != nil && !os.IsNotExist(err) {
				t.Fatal(err)
			}
			for _, id := range ids {
				if strings.Contains(string(log), id) {
					t.Errorf("control %s was also written to control.log", id)
				}
			}
		})
	}
}

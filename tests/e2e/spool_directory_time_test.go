package e2e

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-while/nntp-testkit/client"
)

// dirTimeSlots returns the directory names ("D.%08x", minutes since the epoch) a spool
// directory time of the given reallocint (in minutes) can have for a site that started and
// took its articles within the minutes from..to.
func dirTimeSlots(from, to, interval int64) map[string]bool {
	slots := map[string]bool{}
	for m := from; m <= to; m++ {
		slots[fmt.Sprintf("D.%08x", m-m%interval)] = true
	}
	return slots
}

// articleDir returns the name of the directory below root that holds the spool file of the
// article with message-id id ("" if none does).
func articleDir(t *testing.T, root, id string) string {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(root, "D.*", "B.*"))
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(b), id) {
			return filepath.Base(filepath.Dir(f))
		}
	}
	return ""
}

// TestSpoolUsesMetaspoolDirectoryTime: an article is stored in the directory time its own
// metaspool allocated (reallocint), not the one of the metaspool allocated last. Both
// articles go in on one connection, so the check does not depend on any other connection.
// "distinct": one spool object per metaspool. "shared": both metaspools use spool 01; that layout is still handled (the article lands in its own
// metaspool's slot), but sharing with different reallocint values is not recommended because of
// the expiry guard in dexpire.
func TestSpoolUsesMetaspoolDirectoryTime(t *testing.T) {
	for _, shared := range []bool{false, true} {
		name := "distinct"
		if shared {
			name = "shared"
		}
		t.Run(name, func(t *testing.T) {
			testSpoolDirectoryTime(t, shared)
		})
	}
}

func testSpoolDirectoryTime(t *testing.T, shared bool) {
	// A slow reallocint whose slot starts well before now, so it is a different
	// directory from the ten-minute slots for the whole run.
	minute := time.Now().Unix() / 60
	var slowHours int64
	for _, hours := range []int64{24, 25, 26, 27, 28} {
		offset := minute % (hours * 60)
		if offset >= 20 && offset <= hours*60-20 {
			slowHours = hours
			break
		}
	}
	if slowHours == 0 {
		t.Fatal("could not choose a distinct slow metaspool directory slot")
	}

	slowSpool := "02"
	if shared {
		slowSpool = "01"
	}
	spools := "spool 01\n    minfree 1m\nend\n"
	if !shared {
		spools += "spool 02\n    minfree 1m\nend\n"
	}
	first := time.Now().Unix() / 60
	s := NewSite(t, SiteOptions{
		SpoolCtl: spools + fmt.Sprintf(`metaspool fast
    spool 01
    reallocint 10m
end
metaspool slow
    spool %s
    reallocint %dh
end
expire e2e.fast fast
expire e2e.slow slow
`, slowSpool, slowHours),
		Setup: func(tb testing.TB, s *Site) {
			for _, name := range []string{"P.01", "P.02"} {
				if err := os.MkdirAll(filepath.Join(s.Dir, "spool", "news", name), 0o755); err != nil {
					tb.Fatal(err)
				}
			}
		},
	})

	// Both articles on one connection, in a known time window.
	groups := []string{"e2e.fast", "e2e.slow"}
	ids := make([]string, len(groups))
	p := s.Peer(t)
	for i, g := range groups {
		ids[i] = msgID(t)
		p.ExpectCmd(335, "IHAVE %s", ids[i])
		p.MustSend(client.Stuff(newArticle(ids[i], g, "dirtime "+g, "body of "+ids[i])))
		p.ExpectStatus(235, "IHAVE "+ids[i])
	}
	p.ExpectCmd(205, "QUIT")
	last := time.Now().Unix()/60 + 1

	want := []struct {
		root  string
		slots map[string]bool
	}{
		{"P.01", dirTimeSlots(first, last, 10)},
		{"P." + slowSpool, dirTimeSlots(first, last, slowHours*60)},
	}
	for i, g := range groups {
		root := filepath.Join(s.Dir, "spool", "news", want[i].root)
		got := articleDir(t, root, ids[i])
		if got == "" {
			t.Errorf("%s: article %s not found in %s", g, ids[i], root)
			continue
		}
		if !want[i].slots[got] {
			t.Errorf("%s (%s): article stored in %s, want one of %v", g, want[i].root, got, want[i].slots)
		}
	}
}

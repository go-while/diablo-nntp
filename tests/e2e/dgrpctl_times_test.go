package e2e

import (
	"regexp"
	"strconv"
	"testing"
	"time"
)

// TestDgrpctlGroupinfoTimes: dgrpctl groupinfo prints the creation (CTS) and last-modified
// (LMTS) time of a group as the number and as ctime(3) text. It passed the address of a
// 32-bit int as a time_t *, so ctime read 8 bytes from a 4-byte variable: the high half came
// from whatever followed it on the stack. Where that happens to be zero (gcc -O2 on x86-64) the
// date came out right and this test passes on main; built with -fsanitize=address it fails on
// main (dgrpctl stops with stack-buffer-overflow in ctime).
func TestDgrpctlGroupinfoTimes(t *testing.T) {
	s := NewReaderSite(t, ReaderOptions{
		SpoolAddr: "127.0.0.1:1",
		Groups:    []string{"e2e.test"},
		NoStart:   true,
	})
	s.Run(t, "dgrpctl", "newgroup", "e2e.times", "y", `""`, "Times.")
	out := s.Run(t, "dgrpctl", "groupinfo", "e2e.times")
	re := regexp.MustCompile(`\bCTS=(\d+)=\w{3} \w{3} [ \d]\d \d\d:\d\d:\d\d (\d{4})\n`)
	m := re.FindStringSubmatch(out)
	if m == nil {
		t.Fatalf("no CTS=<seconds>=<ctime text> line in groupinfo:\n%s", out)
	}
	n, _ := strconv.ParseInt(m[1], 10, 64)
	year, _ := strconv.Atoi(m[2])
	if d := time.Since(time.Unix(n, 0)); d < -time.Hour || d > time.Hour {
		t.Errorf("CTS=%d is not the time the group was created (now)", n)
	}
	// ctime is local time; the year may differ from UTC's around new year
	if want := time.Unix(n, 0).UTC().Year(); year < want-1 || year > want+1 {
		t.Errorf("CTS=%d printed as year %d, want %d:\n%s", n, year, want, out)
	}
}

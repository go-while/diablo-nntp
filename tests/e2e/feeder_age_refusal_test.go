package e2e

import (
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"
)

// feederMapCount returns the number of mappings of pid (lines of /proc/pid/maps).
func feederMapCount(t *testing.T, pid int) int {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "maps"))
	if err != nil {
		t.Fatalf("maps of %d: %v", pid, err)
	}
	return strings.Count(string(b), "\n")
}

// feederRSSKiB returns VmRSS of pid in KiB (/proc/pid/status).
func feederRSSKiB(t *testing.T, pid int) int {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "status"))
	if err != nil {
		t.Fatalf("status of %d: %v", pid, err)
	}
	for _, l := range strings.Split(string(b), "\n") {
		if v, ok := strings.CutPrefix(l, "VmRSS:"); ok {
			f := strings.Fields(v)
			if len(f) > 0 {
				if n, err := strconv.Atoi(f[0]); err == nil {
					return n
				}
			}
		}
	}
	t.Fatalf("no VmRSS in /proc/%d/status", pid)
	return 0
}

// TestFeederAgeRefusalReleasesArticle (deep review r2 H3, D-227): "HEAD|BODY|ARTICLE <id>
// a<N>" for an article older than N seconds is answered "430 Article prohibited". The
// feeder has already opened the article at that point: mapped it from a plain spool, or
// inflated it into a malloc'd buffer from a compressed one (compresslvl). Before the fix the
// refusal dropped the pointer without releasing it, so each refused request left one
// mapping, or one heap copy of the article, in the connection's diablo child (dreaderd
// passes a reader's own "a<N>" through, so any reader could repeat it). The refused
// requests of all three commands must leave the child's mapping count and its resident
// memory flat, and the connection must still serve the article without and with a limit it
// passes. The article has a 256 KiB body, so a heap copy per refusal shows in VmRSS.
func TestFeederAgeRefusalReleasesArticle(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("reads /proc/PID/maps and /proc/PID/status")
	}
	for _, spool := range []string{"plain", "compressed"} {
		t.Run(spool, func(t *testing.T) {
			opts := SiteOptions{}
			if spool == "compressed" {
				skipWithoutZlib(t)
				opts.SpoolCtl = compressedSpoolCtl
			}
			s := NewSite(t, opts)
			body := make([]string, 4096)
			for i := range body {
				body[i] = strings.Repeat(string(rune('a'+i%26)), 63)
			}
			id := msgID(t)
			p := s.Peer(t)
			spoolIhave(t, p, id, body...)
			p.Close()
			// the history keeps the arrival in minutes: two seconds make it older than a1
			time.Sleep(2 * time.Second)

			before := sitePids(t, s, "diablo")
			p = s.Peer(t)
			defer p.Close()
			p.ExpectCmd(223, "STAT %s", id)
			child := 0
			for _, pid := range sitePids(t, s, "diablo") {
				if !slices.Contains(before, pid) && pid > child {
					child = pid
				}
			}
			if child == 0 {
				t.Fatalf("no diablo child for the peer connection (before %v)", before)
			}

			// served fetches first, so that what the first fetches set up once is in m0/r0
			for i := 0; i < 3; i++ {
				p.ExpectCmd(220, "ARTICLE %s", id)
				p.ReadDotLines()
			}

			m0, r0 := feederMapCount(t, child), feederRSSKiB(t, child)
			const n = 100
			for _, cmd := range []string{"HEAD", "BODY", "ARTICLE"} {
				for i := 0; i < n; i++ {
					if code, line := p.Cmd("%s %s a1", cmd, id); code != 430 {
						t.Fatalf("request %d: %s %s a1: %d %q, want 430", i+1, cmd, id, code, line)
					}
				}
			}
			m1, r1 := feederMapCount(t, child), feederRSSKiB(t, child)
			t.Logf("%s spool, diablo child %d: %d mappings and %d KiB resident before, %d and %d KiB after %d refused requests",
				spool, child, m0, r0, m1, r1, 3*n)
			if m1-m0 > 20 {
				t.Errorf("%d refused requests left %d more mappings in the diablo child, want none", 3*n, m1-m0)
			}
			// a copy of the article per refusal would be 300 x 256 KiB = 75 MiB
			if r1-r0 > 16*1024 {
				t.Errorf("%d refused requests grew the diablo child's resident memory by %d KiB, want it flat",
					3*n, r1-r0)
			}

			// served fetches on the same connection: no limit, and a limit the article passes
			p.ExpectCmd(220, "ARTICLE %s", id)
			p.ReadDotLines()
			p.ExpectCmd(221, "HEAD %s a999999999", id)
			p.ReadDotLines()
			p.ExpectCmd(222, "BODY %s a999999999", id)
			p.ReadDotLines()
			if m2 := feederMapCount(t, child); m2-m0 > 20 {
				t.Errorf("after the served fetches the diablo child has %d more mappings than before, want none", m2-m0)
			}
		})
	}
}

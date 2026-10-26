package e2e

// The samples/adm maintenance scripts (POSIX sh), run from a site's adm directory with an
// unrelated HOME: they find dbin next to adm and every path through dpath.

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	testkit "github.com/go-while/nntp-testkit"
)

// admSource returns the source tree's samples/adm, or skips while it still holds the old
// csh scripts.
func admSource(tb testing.TB) string {
	tb.Helper()
	dir := filepath.Join(filepath.Dir(bin(tb)), "samples", "adm")
	b, err := os.ReadFile(filepath.Join(dir, "hourly.expire"))
	if err != nil || !bytes.HasPrefix(b, []byte("#!/bin/sh")) {
		tb.Skipf("no POSIX sh adm scripts in %s", dir)
	}
	return dir
}

// withAdm is a SiteOptions.Setup that links dbin and copies the adm scripts into the site.
func withAdm(src string) func(testing.TB, *Site) {
	return func(tb testing.TB, s *Site) {
		tb.Helper()
		linkBin(tb, s)
		dst := filepath.Join(s.Dir, "adm")
		if err := os.MkdirAll(dst, 0o755); err != nil {
			tb.Fatal(err)
		}
		for _, name := range []string{"hourly.expire", "daily.atrim", "biweekly.atrim", "daily.reader", "weekly.reader", "rot"} {
			b, err := os.ReadFile(filepath.Join(src, name))
			if err != nil {
				tb.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dst, name), b, 0o755); err != nil {
				tb.Fatal(err)
			}
		}
	}
}

// runAdm runs <site>/adm/<name> with the site's DIABLO_CONFIG_PATH and HOME set to an empty
// directory elsewhere, and fails the test unless it exits 0 and prints its END line.
func runAdm(tb testing.TB, s *Site, name string, args ...string) string {
	tb.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*testkit.Deadline())
	defer cancel()
	cmd := exec.CommandContext(ctx, filepath.Join(s.Dir, "adm", name), args...)
	cmd.Dir = tb.TempDir() // neither home nor path_home
	cmd.Env = append(os.Environ(), append(s.Env, "HOME="+tb.TempDir())...)
	out, err := cmd.CombinedOutput()
	if err != nil || !strings.Contains(string(out), " END") {
		tb.Fatalf("adm/%s: %v\n%s", name, err, out)
	}
	return string(out)
}

// TestAdmHourlyExpire: adm/hourly.expire expires an article older than keeptime.
func TestAdmHourlyExpire(t *testing.T) {
	src := admSource(t)
	s := NewSite(t, SiteOptions{
		SpoolCtl: "spool 00\n    minfree 1m\n    keeptime 1\nend\nmetaspool trad\n    spool 00\nend\nexpire * trad\n",
		Setup:    withAdm(src),
	})
	id := s.Inject(t, "e2e.test", 1)[0]
	dirs, _ := filepath.Glob(filepath.Join(s.Dir, "spool", "news", "D.*"))
	if len(dirs) == 0 {
		t.Fatal("no spool directory")
	}
	time.Sleep(2 * time.Second)
	// dexpire keeps a spool's newest directory (jrehmer/diablo-nntp#25): add a newer, empty
	// one, as the next time slot gives a real site.
	newer := filepath.Join(s.Dir, "spool", "news", fmt.Sprintf("D.%08x", time.Now().Unix()/60+1))
	if err := os.Mkdir(newer, 0o755); err != nil {
		t.Fatal(err)
	}
	out := runAdm(t, s, "hourly.expire")
	for _, d := range dirs {
		if _, err := os.Stat(d); err == nil {
			t.Errorf("spool directory %s still there after adm/hourly.expire\n%s", d, out)
		}
	}
	p := s.Peer(t)
	expect(t, p, 430, "STAT %s", id)
	expect(t, p, 205, "QUIT")
}

// TestAdmDailyAtrim: adm/daily.atrim rotates the logs (adm/rot).
func TestAdmDailyAtrim(t *testing.T) {
	s := NewSite(t, SiteOptions{Setup: withAdm(admSource(t))})
	logf := func(name string) string { return filepath.Join(s.Dir, "log", name) }
	if err := os.WriteFile(logf("daily.log"), []byte("first\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runAdm(t, s, "daily.atrim")
	if b, _ := os.ReadFile(logf("daily.log.0")); string(b) != "first\n" {
		t.Errorf("daily.log.0 after the first rotation: %q", b)
	}
	if err := os.WriteFile(logf("daily.log"), []byte("second\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runAdm(t, s, "daily.atrim")
	b0, _ := os.ReadFile(logf("daily.log.0"))
	b1, _ := os.ReadFile(logf("daily.log.1"))
	if string(b0) != "second\n" || string(b1) != "first\n" {
		t.Errorf("after the second rotation: daily.log.0 %q, daily.log.1 %q", b0, b1)
	}
}

// TestAdmBiweeklyAtrim: adm/biweekly.atrim trims the history while diablo runs; the stored
// article stays in history.
func TestAdmBiweeklyAtrim(t *testing.T) {
	s := NewSite(t, SiteOptions{Setup: withAdm(admSource(t))})
	id := s.Inject(t, "e2e.test", 1)[0]
	runAdm(t, s, "biweekly.atrim")
	p := s.Peer(t)
	expect(t, p, 435, "IHAVE %s", id) // still in history
	expect(t, p, 223, "STAT %s", id)
	expect(t, p, 205, "QUIT")
}

// TestAdmReaderScripts: adm/daily.reader and adm/weekly.reader run on a reader site.
func TestAdmReaderScripts(t *testing.T) {
	src := admSource(t)
	_, spool := spoolStore(t)
	s := NewReaderSite(t, ReaderOptions{Spool: spool, NoStart: true})
	withAdm(src)(t, s)
	runAdm(t, s, "daily.reader")
	runAdm(t, s, "weekly.reader")
}

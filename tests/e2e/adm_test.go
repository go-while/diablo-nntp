package e2e

// The samples/adm maintenance scripts (POSIX sh), run from a site's adm directory with an
// unrelated HOME: they find dbin next to adm and every path through dpath.

import (
	"bytes"
	"context"
	"errors"
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
	// the new daily.log exists after a rotation (D-138), for the jobs that append to it by name
	if fi, err := os.Stat(logf("daily.log")); err != nil || fi.Size() != 0 {
		t.Errorf("daily.log after the rotation: %v, %v; want an empty file", fi, err)
	}
	b0, _ := os.ReadFile(logf("daily.log.0"))
	b1, _ := os.ReadFile(logf("daily.log.1"))
	if string(b0) != "second\n" || string(b1) != "first\n" {
		t.Errorf("after the second rotation: daily.log.0 %q, daily.log.1 %q", b0, b1)
	}
}

// TestAdmDailyAtrimDaemonLogs: adm/daily.atrim also rotates the logs diablo and dreaderd
// write themselves (general, incoming, drincoming, path, article). They were left out, and
// grew without limit. The daemons open the new file by themselves (lib/log.c checks the
// inode), so a plain rename is enough.
func TestAdmDailyAtrimDaemonLogs(t *testing.T) {
	s := NewSite(t, SiteOptions{Setup: withAdm(admSource(t))})
	names := []string{"general.log", "incoming.log", "drincoming.log", "path.log", "article.log"}
	for _, n := range names {
		if err := os.WriteFile(filepath.Join(s.Dir, "log", n), []byte(n+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	runAdm(t, s, "daily.atrim")
	for _, n := range names {
		b, err := os.ReadFile(filepath.Join(s.Dir, "log", n+".0"))
		if err != nil || string(b) != n+"\n" {
			t.Errorf("%s.0 after the rotation: %q, %v", n, b, err)
		}
		if _, err := os.Stat(filepath.Join(s.Dir, "log", n)); err == nil {
			t.Errorf("%s is still there after the rotation", n)
		}
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

// runAdmStatus runs <site>/adm/<name> with DIABLO_BIN set to bin (a fakeBin, or "" for the
// site's own) and returns its output and exit status.
func runAdmStatus(tb testing.TB, s *Site, bin, name string, args ...string) (string, int) {
	tb.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*testkit.Deadline())
	defer cancel()
	cmd := exec.CommandContext(ctx, filepath.Join(s.Dir, "adm", name), args...)
	cmd.Dir = tb.TempDir()
	cmd.Env = append(os.Environ(), append(s.Env, "HOME="+tb.TempDir())...)
	if bin != "" {
		cmd.Env = append(cmd.Env, "DIABLO_BIN="+bin)
	}
	out, err := cmd.CombinedOutput()
	code := 0
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		code = ee.ExitCode()
	} else if err != nil {
		tb.Fatalf("adm/%s: %v", name, err)
	}
	return string(out), code
}

// TestAdmExitStatus (D-135, D-137): a maintenance job that fails makes its script fail, and the END
// line says with which status; before, the END message hid it and the script exited 0.
func TestAdmExitStatus(t *testing.T) {
	src := admSource(t)
	fail := func(code int) string { return fmt.Sprintf("exit %d\n", code) }
	t.Run("hourly.expire", func(t *testing.T) {
		s := NewSite(t, SiteOptions{Setup: withAdm(src)})
		bin := fakeBin(t, s, map[string]string{"dexpire": fail(7)})
		out, code := runAdmStatus(t, s, bin, "hourly.expire")
		if code != 7 || !strings.Contains(out, "END (exit status 7)") {
			t.Errorf("exit %d, output %q; want 7 and an END line with the status", code, out)
		}
	})
	for _, name := range []string{"daily.reader", "weekly.reader"} {
		t.Run(name, func(t *testing.T) {
			_, spool := spoolStore(t)
			s := NewReaderSite(t, ReaderOptions{Spool: spool, NoStart: true})
			withAdm(src)(t, s)
			bin := fakeBin(t, s, map[string]string{"dexpireover": fail(5)})
			out, code := runAdmStatus(t, s, bin, name)
			if code != 5 || !strings.Contains(out, "END (exit status 5)") {
				t.Errorf("exit %d, output %q; want 5 and an END line with the status", code, out)
			}
			if out, code := runAdmStatus(t, s, "", name); code != 0 || !strings.Contains(out, "END (exit status 0)") {
				t.Errorf("with the real programs: exit %d, output %q; want 0", code, out)
			}
		})
	}
	t.Run("daily.atrim", func(t *testing.T) {
		s := NewSite(t, SiteOptions{Setup: withAdm(src)})
		if err := os.WriteFile(filepath.Join(s.Dir, "adm", "rot"), []byte("#!/bin/sh\nexit 4\n"), 0o755); err != nil {
			t.Fatal(err)
		}
		out, code := runAdmStatus(t, s, "", "daily.atrim")
		if code != 4 || !strings.Contains(out, "END (exit status 4)") {
			t.Errorf("exit %d, output %q; want 4 and an END line with the status", code, out)
		}
	})
	t.Run("biweekly.atrim", func(t *testing.T) {
		s := NewSite(t, SiteOptions{Setup: withAdm(src)})
		s.Inject(t, "e2e.test", 1)
		dp := exec.Command(filepath.Join(s.Bin, "dpath"), "server_dactive")
		dp.Env = append(os.Environ(), s.Env...)
		dactive, err := dp.Output()
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(strings.TrimSpace(string(dactive)), nil, 0o644); err != nil {
			t.Fatal(err)
		}
		// dkp fails after a good history rebuild: the failure is not lost (D-137)
		bin := fakeBin(t, s, map[string]string{"dkp": fail(6)})
		out, code := runAdmStatus(t, s, bin, "biweekly.atrim")
		if code != 6 || !strings.Contains(out, "dkp exit status 6") {
			t.Errorf("failing dkp: exit %d, output %q; want 6 and the dkp status in the END line", code, out)
		}
		// a failing history rebuild is still reported first
		bin = fakeBin(t, s, map[string]string{"dhisexpire": fail(3), "dkp": fail(6)})
		out, code = runAdmStatus(t, s, bin, "biweekly.atrim")
		if code != 3 || !strings.Contains(out, "dhisexpire exit status 3") {
			t.Errorf("failing dhisexpire: exit %d, output %q; want 3", code, out)
		}
		if out, code := runAdmStatus(t, s, "", "biweekly.atrim"); code != 0 || !strings.Contains(out, "dkp exit status 0") {
			t.Errorf("with the real programs: exit %d, output %q; want 0", code, out)
		}
	})
}

// TestAdmRotArguments (D-136): a count that is not a number is refused before any file is moved,
// leading zeros are decimal, and a move that fails makes rot fail.
func TestAdmRotArguments(t *testing.T) {
	rot := filepath.Join(admSource(t), "rot")
	run := func(dir string, args ...string) (string, int) {
		// a count of a billion must be refused at once, not looped over
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, "/bin/sh", append([]string{rot}, args...)...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		code := 0
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			code = ee.ExitCode()
		} else if err != nil {
			t.Fatal(err)
		}
		return string(out), code
	}
	dir := t.TempDir()
	write := func(name, content string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("f", "now")
	write("f.0", "before")
	for _, n := range []string{"", "x", "1x", "-1", "1.5", "1234567890"} {
		out, code := run(dir, n, "f")
		if code != 2 {
			t.Errorf("rot %q f: exit %d, output %q; want 2", n, code, out)
		}
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "f")); string(b) != "now" {
		t.Errorf("f after the refused counts: %q, want it untouched", b)
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "f.0")); string(b) != "before" {
		t.Errorf("f.0 after the refused counts: %q, want it untouched (it was replaced before)", b)
	}
	if out, code := run(dir, "08", "f"); code != 0 {
		t.Fatalf("rot 08 f: exit %d, output %q", code, out)
	}
	for name, want := range map[string]string{"f.0": "now", "f.1": "before"} {
		if b, _ := os.ReadFile(filepath.Join(dir, name)); string(b) != want {
			t.Errorf("%s after rot 08: %q, want %q", name, b, want)
		}
	}
	if os.Geteuid() != 0 { // root moves files into a read-only directory
		ro := t.TempDir()
		if err := os.WriteFile(filepath.Join(ro, "g"), []byte("g"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(ro, 0o555); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { os.Chmod(ro, 0o755) })
		if out, code := run(ro, "2", "g"); code != 1 {
			t.Errorf("rot in a read-only directory: exit %d, output %q; want 1", code, out)
		}
	}
}

package e2e

import (
	"bytes"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// rotRun runs adm/rot in dir with the given rot.conf text (none: no rot.conf).
func rotRun(t *testing.T, dir, conf string, args ...string) (string, int) {
	t.Helper()
	cf := filepath.Join(dir, "rot.conf.test")
	if conf == "" {
		cf = filepath.Join(dir, "no-such-rot.conf")
	} else if err := os.WriteFile(cf, []byte(conf), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("/bin/sh", append([]string{filepath.Join(admSource(t), "rot")}, args...)...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "ROT_CONF="+cf)
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

func rotWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func rotGunzip(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	zr, err := gzip.NewReader(bytes.NewReader(b))
	if err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	out, err := io.ReadAll(zr)
	if err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	return string(out)
}

// TestAdmRotCompress: with "compress gzip" in rot.conf, file.0 stays plain and file.1 and older are
// gzip files, and more than 9 copies can be kept ("keep"/the count): twelve rotations of a log keep
// each of them in order, the thirteenth is dropped.
func TestAdmRotCompress(t *testing.T) {
	if _, err := exec.LookPath("gzip"); err != nil {
		t.Skip("no gzip")
	}
	dir := t.TempDir()
	conf := "keep 12\ncompress gzip\n"
	if out, code := rotRun(t, dir, conf, "-k"); code != 0 || strings.TrimSpace(out) != "12" {
		t.Fatalf("rot -k: exit %d, output %q; want 12", code, out)
	}
	for i := 1; i <= 14; i++ {
		rotWrite(t, filepath.Join(dir, "f"), fmt.Sprintf("day %d\n", i))
		if out, code := rotRun(t, dir, conf, "12", "f"); code != 0 {
			t.Fatalf("rotation %d: exit %d, output %q", i, code, out)
		}
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "f.0")); string(b) != "day 14\n" {
		t.Errorf("f.0 = %q, want day 14 (plain)", b)
	}
	for j := 1; j <= 12; j++ {
		want := fmt.Sprintf("day %d\n", 14-j)
		if got := rotGunzip(t, filepath.Join(dir, fmt.Sprintf("f.%d.gz", j))); got != want {
			t.Errorf("f.%d.gz = %q, want %q", j, got, want)
		}
		if fileExists(filepath.Join(dir, fmt.Sprintf("f.%d", j))) {
			t.Errorf("f.%d is there as well as f.%d.gz", j, j)
		}
	}
	for _, name := range []string{"f.13", "f.13.gz", "f", "f.1.gz.tmp"} {
		if fileExists(filepath.Join(dir, name)) {
			t.Errorf("%s exists", name)
		}
	}
}

// TestAdmRotMethods: a method the admin defines is used; changing the method leaves the copies of
// the old one in place with their own suffix and drops them in turn; a method that cannot run leaves
// the copy plain and makes rot exit 1; a bad rot.conf or an unknown method is refused before any
// file is touched.
func TestAdmRotMethods(t *testing.T) {
	if _, err := exec.LookPath("gzip"); err != nil {
		t.Skip("no gzip")
	}
	dir := t.TempDir()
	path := func(n string) string { return filepath.Join(dir, n) }
	rot := func(conf string, n string) (string, int) { return rotRun(t, dir, conf, n, "f") }

	// own method: tr as the "compressor", suffix .up
	up := "keep 3\ncompress upper\nmethod upper .up tr a-z A-Z\n"
	rotWrite(t, path("f"), "one\n")
	rot(up, "3")
	rotWrite(t, path("f"), "two\n")
	if out, code := rot(up, "3"); code != 0 {
		t.Fatalf("own method: exit %d, output %q", code, out)
	}
	if b, _ := os.ReadFile(path("f.1.up")); string(b) != "ONE\n" {
		t.Errorf("f.1.up = %q, want ONE", b)
	}
	// now gzip: the .up copy is rotated as it is, the new one is .gz
	gz := "compress gzip\nmethod upper .up tr a-z A-Z\n" // the old method stays defined while its copies exist
	rotWrite(t, path("f"), "three\n")
	if out, code := rot(gz, "3"); code != 0 {
		t.Fatalf("switch to gzip: exit %d, output %q", code, out)
	}
	if b, _ := os.ReadFile(path("f.2.up")); string(b) != "ONE\n" {
		t.Errorf("f.2.up = %q, want ONE (rotated with its own suffix)", b)
	}
	if got := rotGunzip(t, path("f.1.gz")); got != "two\n" {
		t.Errorf("f.1.gz = %q, want two", got)
	}
	// the old .up copy is dropped at the end of the line, whatever the method
	for _, c := range []string{"four\n", "five\n"} {
		rotWrite(t, path("f"), c)
		rot(gz, "3")
	}
	if fileExists(path("f.4.up")) || fileExists(path("f.3.up")) || fileExists(path("f.2.up")) {
		t.Errorf("an .up copy outlived the count of 3")
	}

	// a method that fails: the copy stays plain, exit 1
	bad := "compress broken\nmethod broken .bad false\n"
	rotWrite(t, path("f"), "six\n")
	rot(bad, "3")
	rotWrite(t, path("f"), "seven\n")
	out, code := rot(bad, "3")
	if code != 1 || !strings.Contains(out, "left uncompressed") {
		t.Errorf("failing method: exit %d, output %q; want 1 and a message", code, out)
	}
	if b, _ := os.ReadFile(path("f.1")); string(b) != "six\n" {
		t.Errorf("f.1 = %q, want six (plain)", b)
	}
	if fileExists(path("f.1.bad")) || fileExists(path("f.1.bad.tmp")) {
		t.Errorf("a half-made f.1.bad is left")
	}

	// refused before anything is moved
	rotWrite(t, path("f"), "keep me\n")
	for name, conf := range map[string]string{
		"unknown method":  "compress nosuch\n",
		"unknown line":    "frobnicate 1\n",
		"keep not number": "keep many\n",
		"suffix no dot":   "method x gz gzip -c\n",
		"suffix slash":    "method x .a/b gzip -c\n",
		"method too few":  "method x .x\n",
	} {
		if out, code := rot(conf, "3"); code != 2 {
			t.Errorf("%s: exit %d, output %q; want 2", name, code, out)
		}
		if b, _ := os.ReadFile(path("f")); string(b) != "keep me\n" {
			t.Errorf("%s: f = %q, want it untouched", name, b)
		}
	}
}

// TestAdmDailyAtrimKeep: daily.atrim keeps the number of copies of rot.conf and compresses them.
func TestAdmDailyAtrimKeep(t *testing.T) {
	if _, err := exec.LookPath("gzip"); err != nil {
		t.Skip("no gzip")
	}
	s := NewSite(t, SiteOptions{Setup: withAdm(admSource(t))})
	rotWrite(t, filepath.Join(s.Dir, "adm", "rot.conf"), "keep 2\ncompress gzip\n")
	logf := func(n string) string { return filepath.Join(s.Dir, "log", n) }
	for i := 1; i <= 4; i++ {
		rotWrite(t, logf("general.log"), fmt.Sprintf("day %d\n", i))
		runAdm(t, s, "daily.atrim")
	}
	if b, _ := os.ReadFile(logf("general.log.0")); string(b) != "day 4\n" {
		t.Errorf("general.log.0 = %q, want day 4", b)
	}
	if got := rotGunzip(t, logf("general.log.2.gz")); got != "day 2\n" {
		t.Errorf("general.log.2.gz = %q, want day 2", got)
	}
	if fileExists(logf("general.log.3.gz")) || fileExists(logf("general.log.3")) {
		t.Errorf("general.log.3 exists: more than keep 2 kept")
	}
}

// TestAdmRotSample: the sample rot.conf, which the install copies next to rot, changes nothing
// (4 copies, plain): its lines are comments, the examples too. Before, it asked for 14 copies and
// gzip, so installing it changed the rotation of a site that had no rot.conf.
func TestAdmRotSample(t *testing.T) {
	sample := filepath.Join(admSource(t), "rot.conf")
	b, err := os.ReadFile(sample)
	if err != nil {
		t.Fatal(err)
	}
	for i, line := range strings.Split(string(b), "\n") {
		if l := strings.TrimSpace(line); l != "" && !strings.HasPrefix(l, "#") {
			t.Errorf("rot.conf:%d: %q is not a comment", i+1, line)
		}
	}
	dir := t.TempDir()
	if out, code := rotRun(t, dir, string(b), "-k"); code != 0 || strings.TrimSpace(out) != "4" {
		t.Errorf("rot -k with the sample: exit %d, output %q; want 4", code, out)
	}
	for _, c := range []string{"one\n", "two\n"} {
		rotWrite(t, filepath.Join(dir, "f"), c)
		if out, code := rotRun(t, dir, string(b), "4", "f"); code != 0 {
			t.Fatalf("rot 4 f with the sample: exit %d, output %q", code, out)
		}
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "f.1")); string(b) != "one\n" {
		t.Errorf("f.1 = %q, want one (plain)", b)
	}
}

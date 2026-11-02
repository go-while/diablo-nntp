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
	"slices"
	"strings"
	"testing"
)

// rotRun runs adm/rot in dir with the given rot.conf text (none: an empty rot.conf).
func rotRun(t *testing.T, dir, conf string, args ...string) (string, int) {
	t.Helper()
	cf := filepath.Join(dir, "rot.conf.test")
	if conf == "" {
		cf = os.DevNull
	} else if err := os.WriteFile(cf, []byte(conf), 0o644); err != nil {
		t.Fatal(err)
	}
	return rotRunConf(t, dir, cf, args...)
}

// rotRunConf runs adm/rot in dir with ROT_CONF set to cf.
func rotRunConf(t *testing.T, dir, cf string, args ...string) (string, int) {
	t.Helper()
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
	if out, code := rot(up, "3"); code != 0 {
		t.Fatalf("first rotation: exit %d, output %q", code, out)
	}
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
		if out, code := rot(gz, "3"); code != 0 {
			t.Fatalf("rotation of %q: exit %d, output %q", c, code, out)
		}
	}
	if fileExists(path("f.4.up")) || fileExists(path("f.3.up")) || fileExists(path("f.2.up")) {
		t.Errorf("an .up copy outlived the count of 3")
	}

	// a method that fails: the copy stays plain, exit 1
	bad := "compress broken\nmethod broken .bad false\n"
	rotWrite(t, path("f"), "six\n")
	if out, code := rot(bad, "3"); code != 1 { // the f.0 of five fails too
		t.Fatalf("rotation of six: exit %d, output %q; want 1", code, out)
	}
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

// TestAdmRotConfigErrors: a rot.conf that is there but cannot be read, a ROT_CONF that names no
// file, a method line that redefines a built-in method, a keep above 9999 and an unknown compress
// method (with "rot -k" too, which daily.atrim runs first) are refused with 2 before any file is
// touched. Before, an unreadable rot.conf gave a warning and the defaults, exit 0; ROT_CONF without
// a file meant the defaults; "rot -k" printed the count without checking the method.
func TestAdmRotConfigErrors(t *testing.T) {
	dir := t.TempDir()
	path := func(n string) string { return filepath.Join(dir, n) }
	untouched := func(what string) {
		t.Helper()
		if b, _ := os.ReadFile(path("f")); string(b) != "keep me\n" {
			t.Errorf("%s: f = %q, want it untouched", what, b)
		}
		if fileExists(path("f.0")) {
			t.Errorf("%s: f.0 was made", what)
		}
	}
	rotWrite(t, path("f"), "keep me\n")

	if os.Geteuid() != 0 { // root reads a file of mode 000
		cf := path("rot.conf.locked")
		rotWrite(t, cf, "keep 9\n")
		if err := os.Chmod(cf, 0); err != nil {
			t.Fatal(err)
		}
		for _, args := range [][]string{{"-k"}, {"3", "f"}} {
			if out, code := rotRunConf(t, dir, cf, args...); code != 2 {
				t.Errorf("unreadable rot.conf, rot %v: exit %d, output %q; want 2", args, code, out)
			}
		}
		untouched("unreadable rot.conf")
	}
	for _, args := range [][]string{{"-k"}, {"3", "f"}} {
		if out, code := rotRunConf(t, dir, path("no-such-rot.conf"), args...); code != 2 {
			t.Errorf("ROT_CONF without a file, rot %v: exit %d, output %q; want 2", args, code, out)
		}
	}
	untouched("ROT_CONF without a file")

	for name, conf := range map[string]string{
		"gzip redefined":     "method gzip .gz gzip -9 -c\n",
		"none redefined":     "method none .n cat\n",
		"compress redefined": "method compress .Z compress -c\n",
		"keep over 9999":     "keep 10000\n",
		"unknown method":     "keep 3\ncompress nosuch\n",
	} {
		for _, args := range [][]string{{"-k"}, {"3", "f"}} {
			if out, code := rotRun(t, dir, conf, args...); code != 2 {
				t.Errorf("%s, rot %v: exit %d, output %q; want 2", name, args, code, out)
			}
		}
		untouched(name)
	}
	for conf, want := range map[string]string{"keep 9999\n": "9999", "keep 0\n": "0", "keep 007\n": "7"} {
		if out, code := rotRun(t, dir, conf, "-k"); code != 0 || strings.TrimSpace(out) != want {
			t.Errorf("%q, rot -k: exit %d, output %q; want %s", conf, code, out, want)
		}
	}
}

// TestAdmRotKeepLowered: when keep is lowered, the copies above the new count go at the next
// rotation, plain or with any suffix rot knows (built in or a method of rot.conf); names that only
// look like copies (a leading zero, an unknown suffix, a .tmp, another file, a directory) stay.
// Before, rot only looked at file.0 to file.<n>, and the copies above stayed for ever.
func TestAdmRotKeepLowered(t *testing.T) {
	if _, err := exec.LookPath("gzip"); err != nil {
		t.Skip("no gzip")
	}
	dir := t.TempDir()
	path := func(n string) string { return filepath.Join(dir, n) }
	conf := "keep 5\ncompress gzip\nmethod upper .up tr a-z A-Z\n"
	for i := 1; i <= 7; i++ {
		rotWrite(t, path("f"), fmt.Sprintf("day %d\n", i))
		if out, code := rotRun(t, dir, conf, "5", "f"); code != 0 {
			t.Fatalf("rotation %d: exit %d, output %q", i, code, out)
		}
	}
	// copies of earlier settings: plain, another built-in method, a method of rot.conf
	for _, n := range []string{"f.3", "f.4.up", "f.5.bz2", "f.10.xz"} {
		rotWrite(t, path(n), "old\n")
	}
	keepThese := []string{"f.06", "f.3.bak", "f.7.gz.tmp", "fz.3", "f.x", "f.3gz"}
	for _, n := range keepThese {
		rotWrite(t, path(n), "not a copy\n")
	}
	if err := os.Mkdir(path("f.9"), 0o755); err != nil {
		t.Fatal(err)
	}
	keepThese = append(keepThese, "f.9")

	rotWrite(t, path("f"), "day 8\n")
	if out, code := rotRun(t, dir, strings.Replace(conf, "keep 5", "keep 1", 1), "1", "f"); code != 0 {
		t.Fatalf("rotation with keep 1: exit %d, output %q", code, out)
	}
	if b, _ := os.ReadFile(path("f.0")); string(b) != "day 8\n" {
		t.Errorf("f.0 = %q, want day 8", b)
	}
	if got := rotGunzip(t, path("f.1.gz")); got != "day 7\n" {
		t.Errorf("f.1.gz = %q, want day 7", got)
	}
	ents, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range ents {
		n := e.Name()
		if strings.HasPrefix(n, "f.") && n != "f.0" && n != "f.1.gz" && !slices.Contains(keepThese, n) {
			t.Errorf("%s is still there after keep was lowered to 1", n)
		}
	}
	for _, n := range keepThese {
		if _, err := os.Stat(path(n)); err != nil {
			t.Errorf("%s is gone, but it is not a copy rot made: %v", n, err)
		}
	}
}

// TestAdmRotCompressLast: the copies are compressed after every file has been rotated, so a live
// log is not held up by the compression of the logs before it; and the compressor's own messages
// reach standard error (cron mails them) when it fails. Before, f.1 was compressed before g was
// renamed, and the compressor's standard error went to /dev/null.
func TestAdmRotCompressLast(t *testing.T) {
	dir := t.TempDir()
	path := func(n string) string { return filepath.Join(dir, n) }
	bin := t.TempDir()
	// the "compressor" notes whether g has been rotated already, then copies its input
	check := filepath.Join(bin, "check")
	rotWrite(t, check, "#!/bin/sh\nif [ \"$(cat g.0 2>/dev/null)\" = \"g new\" ]; then echo yes >> order; else echo no >> order; fi\ncat\n")
	warn := filepath.Join(bin, "warn")
	rotWrite(t, warn, "#!/bin/sh\necho 'warn: no space left on device' >&2\nexit 1\n")
	for _, p := range []string{check, warn} {
		if err := os.Chmod(p, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	conf := "compress check\nmethod check .chk " + check + "\n"
	for _, v := range []string{"old", "new"} {
		rotWrite(t, path("f"), "f "+v+"\n")
		rotWrite(t, path("g"), "g "+v+"\n")
		if out, code := rotRun(t, dir, conf, "2", "f", "g"); code != 0 {
			t.Fatalf("rot 2 f g (%s): exit %d, output %q", v, code, out)
		}
	}
	if b, _ := os.ReadFile(path("order")); string(b) != "yes\nyes\n" {
		t.Errorf("g rotated before each compression: %q, want yes twice", b)
	}
	if b, _ := os.ReadFile(path("f.1.chk")); string(b) != "f old\n" {
		t.Errorf("f.1.chk = %q, want f old", b)
	}

	conf = "compress warn\nmethod warn .w " + warn + "\n"
	rotWrite(t, path("f"), "f newer\n")
	if out, code := rotRun(t, dir, conf, "2", "f"); code != 1 || !strings.Contains(out, "no space left on device") || !strings.Contains(out, "left uncompressed") {
		t.Errorf("failing compressor: exit %d, output %q; want 1, its message and rot's", code, out)
	}
	if b, _ := os.ReadFile(path("f.1")); string(b) != "f new\n" {
		t.Errorf("f.1 = %q, want f new (plain)", b)
	}
	if fileExists(path("f.1.w")) || fileExists(path("f.1.w.tmp")) {
		t.Errorf("a half-made f.1.w is left")
	}
}

// TestAdmRotCompressGrows: the built-in compress method runs "compress -f -c". ncompress and the BSD
// compress exit 2 without -f when the output is not smaller than the input (an empty or a small log),
// and the BSD one also refuses to write to an existing output file then; rot took that for a failure
// and left the copy plain every day. A stand-in compress that behaves so checks the flag.
func TestAdmRotCompressGrows(t *testing.T) {
	bin := t.TempDir()
	rotWrite(t, filepath.Join(bin, "compress"), "#!/bin/sh\ncat\ncase \" $* \" in *\" -f \"*) exit 0 ;; esac\nexit 2\n")
	if err := os.Chmod(filepath.Join(bin, "compress"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	dir := t.TempDir()
	for _, c := range []string{"", "x\n"} {
		rotWrite(t, filepath.Join(dir, "f"), c)
		if out, code := rotRun(t, dir, "compress compress\n", "2", "f"); code != 0 {
			t.Fatalf("rot 2 f with compress: exit %d, output %q", code, out)
		}
	}
	if !fileExists(filepath.Join(dir, "f.1.Z")) || fileExists(filepath.Join(dir, "f.1")) {
		t.Errorf("f.1.Z missing or f.1 left plain")
	}
}

// TestAdmRotCompressMode: the compressed copy keeps the mode of the log, as the rename of a plain
// copy does, whatever the umask of rot: 0600 and 0640 under umask 022, 0644 under umask 077. Before,
// the copy was made with the umask (0600 -> 0644 under 022, 0644 -> 0600 under 077).
func TestAdmRotCompressMode(t *testing.T) {
	if _, err := exec.LookPath("gzip"); err != nil {
		t.Skip("no gzip")
	}
	rot := filepath.Join(admSource(t), "rot")
	for _, c := range []struct {
		mode  os.FileMode
		umask string
	}{{0o600, "022"}, {0o640, "022"}, {0o644, "077"}} {
		dir := t.TempDir()
		f := filepath.Join(dir, "f")
		for _, day := range []string{"one\n", "two\n"} {
			rotWrite(t, f, day)
			if err := os.Chmod(f, c.mode); err != nil {
				t.Fatal(err)
			}
			cmd := exec.Command("/bin/sh", "-c", "umask "+c.umask+"; exec /bin/sh \"$0\" \"$@\"", rot, "3", "f")
			cmd.Dir = dir
			cmd.Env = append(os.Environ(), "ROT_CONF="+filepath.Join(dir, "rot.conf.test"))
			rotWrite(t, filepath.Join(dir, "rot.conf.test"), "compress gzip\n")
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("mode %o, umask %s: %v\n%s", c.mode, c.umask, err, out)
			}
		}
		fi, err := os.Stat(f + ".1.gz")
		if err != nil {
			t.Fatal(err)
		}
		if got := fi.Mode().Perm(); got != c.mode {
			t.Errorf("log of mode %o, umask %s: f.1.gz has mode %o, want %o", c.mode, c.umask, got, c.mode)
		}
		if got := rotGunzip(t, f+".1.gz"); got != "one\n" {
			t.Errorf("f.1.gz = %q, want one", got)
		}
	}
}

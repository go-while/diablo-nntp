package e2e

import (
	"bytes"
	"encoding/binary"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"os/exec"
	"testing"
)

// overSite is a reader site (not started) with one group e2e.test whose article range is
// 1..300, and the hierarchy layout of overview files: <group home>/e2e/o.0.e2e.test and
// d.<artBase>.e2e.test[.gz].
func overSite(t *testing.T, config map[string]string) (s *Site, home string) {
	t.Helper()
	s = NewReaderSite(t, ReaderOptions{
		SpoolAddr: "127.0.0.1:1",
		Groups:    []string{"e2e.test"},
		Config:    config,
		NoStart:   true,
	})
	s.Run(t, "dgrpctl", "adjust", "e2e.test", "NB", "=1")
	s.Run(t, "dgrpctl", "adjust", "e2e.test", "NE", "=300")
	home = strings.TrimSpace(s.Run(t, "dpath", "group"))
	if err := os.MkdirAll(filepath.Join(home, "e2e"), 0o755); err != nil {
		t.Fatal(err)
	}
	return s, home
}

// overHead returns an OverHead (dreaderd/defs.h: version 4, 256 entries per data file) for
// group, in the byte order of this host.
func overHead(group string) []byte {
	b := make([]byte, 16+256+4)
	o := binary.NativeEndian
	o.PutUint32(b[0:], 4)
	o.PutUint32(b[4:], 0xF1E2D3C4)
	o.PutUint32(b[8:], uint32(len(b)))
	o.PutUint32(b[12:], 1000)
	copy(b[16:16+256], group)
	o.PutUint32(b[16+256:], 256)
	return b
}

func writeOver(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

// files lists the regular files and fifos below dir, as paths relative to it.
func filesBelow(t *testing.T, dir string) []string {
	t.Helper()
	var out []string
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			rel, _ := filepath.Rel(dir, p)
			out = append(out, rel)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// TestDoverctlConvertMovesCompressedData (D-150): doverctl convert moves the compressed
// (.gz) data files together with the plain ones. It moved only the plain names, so after a
// hash switch the compressed files stayed behind in the old layout while the overview index
// had moved, and the data was unreachable.
func TestDoverctlConvertMovesCompressedData(t *testing.T) {
	s, home := overSite(t, nil)
	writeOver(t, filepath.Join(home, "e2e", "o.0.e2e.test"), overHead("e2e.test"))
	writeOver(t, filepath.Join(home, "e2e", "d.0.e2e.test"), []byte("plain"))
	writeOver(t, filepath.Join(home, "e2e", "d.256.e2e.test.gz"), []byte("gzipped"))

	out := s.Run(t, "doverctl", "-v", "convert", "hierarchy", "crc")
	if !strings.Contains(out, "Converted e2e.test") {
		t.Fatalf("group not converted:\n%s", out)
	}
	left, _ := filepath.Glob(filepath.Join(home, "e2e", "*"))
	if len(left) != 0 {
		t.Errorf("files left in the old layout: %q\n%s", left, out)
	}
	var gz, other int
	for _, f := range filesBelow(t, home) {
		if strings.HasSuffix(f, ".gz") {
			gz++
		} else {
			other++
		}
	}
	if gz != 1 || other != 2 {
		t.Errorf("after conversion: %d compressed and %d other files, want 1 and 2 (over file and one data file)", gz, other)
	}
}

// TestDoverctlConvertCopyFailure (D-151): with -m a data file that cannot be copied stops the
// conversion of the group, with a message, and the group is not reported as converted. The
// result of the copy was ignored.
func TestDoverctlConvertCopyFailure(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root can read any file")
	}
	s, home := overSite(t, nil)
	writeOver(t, filepath.Join(home, "e2e", "o.0.e2e.test"), overHead("e2e.test"))
	data := filepath.Join(home, "e2e", "d.0.e2e.test")
	writeOver(t, data, []byte("plain"))
	if err := os.Chmod(data, 0); err != nil {
		t.Fatal(err)
	}
	out := s.Run(t, "doverctl", "-v", "-m", "convert", "hierarchy", "crc")
	if strings.Contains(out, "Converted e2e.test") {
		t.Errorf("incomplete conversion reported as converted:\n%s", out)
	}
	if !strings.Contains(out, "Cannot move") {
		t.Errorf("the failed move was not reported:\n%s", out)
	}
}

// TestDoverctlCleanRemovesUnknownEntries (D-152): clean removes a file that is neither
// regular nor a directory by its own path. It looked for <dir>/<dir>/<name>, did not find it,
// and still counted the file as removed.
func TestDoverctlCleanRemovesUnknownEntries(t *testing.T) {
	s, home := overSite(t, nil)
	fifo := filepath.Join(home, "e2e", "junk")
	if err := exec.Command("mkfifo", fifo).Run(); err != nil {
		t.Skipf("no fifo: %v", err)
	}
	out := s.Run(t, "doverctl", "-v", "clean")
	if _, err := os.Lstat(fifo); err == nil {
		t.Errorf("the unknown entry is still there:\n%s", out)
	}
}

// TestDoverctlCleanMisalignedData (D-153): clean applies dexpireover's criteria, so a data
// file whose first article number is not a multiple of the entries per file (left over from
// an entry-size change) is removed, a properly aligned one in the group's range stays.
func TestDoverctlCleanMisalignedData(t *testing.T) {
	s, home := overSite(t, map[string]string{"readergrouphash": "hierarchy"})
	writeOver(t, filepath.Join(home, "e2e", "o.0.e2e.test"), overHead("e2e.test"))
	bad := filepath.Join(home, "e2e", "d.100.e2e.test")
	good := filepath.Join(home, "e2e", "d.256.e2e.test")
	writeOver(t, bad, []byte("misaligned"))
	writeOver(t, good, bytes.Repeat([]byte("x"), 10))
	out := s.Run(t, "doverctl", "-v", "clean")
	if _, err := os.Stat(bad); err == nil {
		t.Errorf("the misaligned data file is still there:\n%s", out)
	}
	if _, err := os.Stat(good); err != nil {
		t.Errorf("the aligned data file was removed:\n%s", out)
	}
}

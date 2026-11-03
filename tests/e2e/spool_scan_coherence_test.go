package e2e

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

func checkSpoolOffsets(t *testing.T, out string, ids []string) {
	t.Helper()
	var offsets []int
	for _, line := range strings.Split(out, "\n") {
		i := strings.Index(line, "SOUT\t")
		if i < 0 {
			continue
		}
		line = line[i:]
		fields := strings.Split(line, "\t")
		if len(fields) < 4 || fields[0] != "SOUT" {
			continue
		}
		parts := strings.Split(fields[2], ",")
		n, err := strconv.Atoi(parts[0])
		if err != nil {
			t.Fatalf("invalid spool offset: %q", line)
		}
		offsets = append(offsets, n)
	}
	if len(offsets) != len(ids) || offsets[0] != 0 || offsets[1] <= offsets[0] {
		t.Errorf("spool offsets %v, want two increasing offsets from zero:\n%s", offsets, out)
	}
}

func runSpoolScan(t *testing.T, s *Site, file string) (string, error) {
	t.Helper()
	dir := filepath.Dir(filepath.Dir(file))
	rel := filepath.Join(filepath.Base(filepath.Dir(file)), filepath.Base(file))
	cmd := exec.Command(filepath.Join(s.Bin, "diloadfromspool"), "-Q", rel)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), s.Env...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func scanSpoolForTest(t *testing.T, s *Site, file string) string {
	t.Helper()
	out, err := runSpoolScan(t, s, file)
	if err != nil {
		t.Fatalf("diloadfromspool: %v\n%s", err, out)
	}
	return out
}

func TestSpoolScanMultipleArticles(t *testing.T) {
	for _, tc := range []struct {
		name string
		ctl  string
	}{
		{"plain", ""},
		{"gzip", compressedSpoolCtl},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.name == "gzip" {
				skipWithoutZlib(t)
			}
			s := NewSite(t, SiteOptions{SpoolCtl: tc.ctl})
			c := s.Peer(t)
			ids := []string{msgID(t), msgID(t)}
			for _, id := range ids {
				expect(t, c, 335, "IHAVE %s", id)
				send(t, c, wire(newArticle(id, "e2e.test", "scan test", "body")))
				status(t, c, 235, "IHAVE "+id)
			}
			files := spoolFiles(t, s)
			if len(files) != 1 {
				t.Fatalf("spool files: %q", files)
			}
			out := scanSpoolForTest(t, s, files[0])
			for _, id := range ids {
				if !strings.Contains(out, id) {
					t.Errorf("missing %s in scan:\n%s", id, out)
				}
			}
			checkSpoolOffsets(t, out, ids)
			expect(t, c, 205, "QUIT")
		})
	}
}

func TestSpoolScanOldFormat(t *testing.T) {
	s := NewSite(t, SiteOptions{NoStart: true})
	dir := filepath.Join(t.TempDir(), "D.00000001")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	ids := []string{msgID(t), msgID(t)}
	var data strings.Builder
	for _, id := range ids {
		data.WriteString("Message-ID: " + id + "\nNewsgroups: e2e.test\nLines: 1\n\nbody\n\x00")
	}
	file := filepath.Join(dir, "B.0001")
	if err := os.WriteFile(file, []byte(data.String()), 0o600); err != nil {
		t.Fatal(err)
	}
	out := scanSpoolForTest(t, s, file)
	for _, id := range ids {
		if !strings.Contains(out, id) {
			t.Errorf("missing %s in old-format scan:\n%s", id, out)
		}
	}
	checkSpoolOffsets(t, out, ids)
}

func TestSpoolScanTruncatedHeader(t *testing.T) {
	if runtime.GOOS != "openbsd" {
		t.Skip("OpenBSD bounded scanner")
	}
	s := NewSite(t, SiteOptions{NoStart: true})
	dir := filepath.Join(t.TempDir(), "D.00000001")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(dir, "B.0001")
	if err := os.WriteFile(file, []byte{0xff, 0x99}, 0o600); err != nil {
		t.Fatal(err)
	}
	out, err := runSpoolScan(t, s, file)
	if err == nil {
		t.Error("truncated spool scan unexpectedly succeeded")
	}
	if !strings.Contains(out, "Incomplete spool header") {
		t.Errorf("truncated header not reported: %q", out)
	}
}

package e2e

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"testing"
)

func TestFeedStatsSnapshotSeesMappedUpdates(t *testing.T) {
	s := NewSite(t, SiteOptions{Config: map[string]string{"feederrtstats": "label"}})
	first := s.Peer(t)
	second := s.Peer(t)
	want := feedStatsCon(t, s, "-i", "peer")
	if want != 2 {
		t.Fatalf("live connection count: got %d, want 2", want)
	}
	s.Run(t, "dfeedinfo", "-Pcoherence")
	cfg, err := os.ReadFile(filepath.Join(s.Dir, "diablo.config"))
	if err != nil {
		t.Fatal(err)
	}
	cfg = append(cfg, []byte("\npath_feedstats "+filepath.Join(s.Dir, "feedstats.coherence")+"\n")...)
	configPath := filepath.Join(s.Dir, "snapshot.config")
	if err := os.WriteFile(configPath, cfg, 0o600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(filepath.Join(s.Bin, "dfeedinfo"), "-i", "-r", "peer")
	cmd.Env = append(os.Environ(), "DIABLO_CONFIG_PATH="+configPath)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("snapshot dfeedinfo: %v\n%s", err, out)
	}
	m := regexp.MustCompile(`(?m)^\S+ peer .* con=(\d+) `).FindStringSubmatch(string(out))
	if m == nil {
		t.Fatalf("peer missing from snapshot:\n%s", out)
	}
	got, _ := strconv.Atoi(m[1])
	if got != want {
		t.Fatalf("snapshot connection count %d, want %d", got, want)
	}
	expect(t, first, 205, "QUIT")
	expect(t, second, 205, "QUIT")
}

package e2e

// install-portable.sh with DESTDIR (no root needed): configuration files are real files
// that a reinstall never overwrites, adm scripts are executable, old config symlinks into
// samples/ become real copies.

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	testkit "github.com/go-while/nntp-testkit"
)

// sourceRoot returns the source tree of the build in DIABLO_BIN (its parent), or skips.
func sourceRoot(tb testing.TB) string {
	tb.Helper()
	root := filepath.Dir(bin(tb))
	if _, err := os.Stat(filepath.Join(root, "install-portable.sh")); err != nil {
		tb.Skipf("no install-portable.sh next to %s", bin(tb))
	}
	return root
}

// install runs install-portable.sh <mode> /news <stage> in the source tree.
func install(tb testing.TB, root, mode, stage string) string {
	tb.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), testkit.Deadline())
	defer cancel()
	cmd := exec.CommandContext(ctx, "/bin/sh", "./install-portable.sh", mode, "/news", stage)
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	if err != nil {
		tb.Fatalf("install-portable.sh %s: %v\n%s", mode, err, out)
	}
	return string(out)
}

func fileMode(tb testing.TB, path string) os.FileMode {
	tb.Helper()
	st, err := os.Lstat(path)
	if err != nil {
		tb.Fatal(err)
	}
	return st.Mode()
}

func TestInstallConfigFiles(t *testing.T) {
	root := sourceRoot(t)
	stage := t.TempDir()
	news := filepath.Join(stage, "news")
	install(t, root, "install", stage)

	// Real files, readable only by owner and group, with the sample's content.
	for _, name := range []string{"diablo.config", "dnewsfeeds", "dreader.access", "dspool.ctl"} {
		p := filepath.Join(news, name)
		m := fileMode(t, p)
		if !m.IsRegular() || m.Perm() != 0o640 {
			t.Errorf("%s: mode %v, want a regular file with 0640", name, m)
		}
		want, _ := os.ReadFile(filepath.Join(root, "samples", name))
		if got, _ := os.ReadFile(p); !bytes.Equal(got, want) {
			t.Errorf("%s: content differs from samples/%s", name, name)
		}
	}
	if st, err := os.Stat(filepath.Join(news, "dactive.kp")); err != nil || st.Size() != 0 {
		t.Errorf("dactive.kp: %v, size %v; want an empty file", err, st)
	}

	// Logs and run files name clients and users: the news user and group only.
	for _, dir := range []string{"log", "run"} {
		if m := fileMode(t, filepath.Join(news, dir)); !m.IsDir() || m.Perm() != 0o750 {
			t.Errorf("%s: mode %v, want a directory with 0750", dir, m)
		}
	}
	if m := fileMode(t, filepath.Join(news, "dqueue")); m.Perm() != 0o755 {
		t.Errorf("dqueue: mode %v, want 0755", m)
	}

	// adm scripts: executable where the source is; the reference copy keeps the bit too.
	adm, _ := filepath.Glob(filepath.Join(root, "samples", "adm", "*"))
	execs := 0
	for _, src := range adm {
		st, err := os.Stat(src)
		if err != nil || !st.Mode().IsRegular() {
			continue
		}
		name := filepath.Base(src)
		want := os.FileMode(0o644)
		if st.Mode()&0o111 != 0 {
			want = 0o755
			execs++
			if m := fileMode(t, filepath.Join(news, "samples", "adm", name)); m.Perm() != 0o555 {
				t.Errorf("samples/adm/%s: mode %v, want 0555", name, m.Perm())
			}
		}
		if m := fileMode(t, filepath.Join(news, "adm", name)); m.Perm() != want {
			t.Errorf("adm/%s: mode %v, want %v", name, m.Perm(), want)
		}
	}
	if execs == 0 {
		t.Errorf("no executable script in samples/adm: test can't check the execute bit")
	}

	// A reinstall keeps local changes: the config file as it is, a changed adm script with
	// the new version next to it.
	local := []byte("# local change\n")
	if err := os.WriteFile(filepath.Join(news, "diablo.config"), local, 0o640); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(news, "adm", "rot"), local, 0o755); err != nil {
		t.Fatal(err)
	}
	install(t, root, "install", stage)
	if got, _ := os.ReadFile(filepath.Join(news, "diablo.config")); !bytes.Equal(got, local) {
		t.Errorf("diablo.config overwritten by a reinstall: %q", got)
	}
	if got, _ := os.ReadFile(filepath.Join(news, "adm", "rot")); !bytes.Equal(got, local) {
		t.Errorf("changed adm/rot overwritten by a reinstall")
	}
	if _, err := os.Stat(filepath.Join(news, "adm", "rot.new")); err != nil {
		t.Errorf("no adm/rot.new next to the changed script: %v", err)
	}

	// Uninstall leaves the configuration.
	install(t, root, "uninstall", stage)
	if _, err := os.Stat(filepath.Join(news, "diablo.config")); err != nil {
		t.Errorf("uninstall removed diablo.config: %v", err)
	}
	if _, err := os.Stat(filepath.Join(news, "dbin", "diablo")); err == nil {
		t.Errorf("uninstall left dbin/diablo")
	}
}

// TestInstallConvertsConfigSymlinks: an earlier install's symlink /news/dnewsfeeds ->
// /news/samples/dnewsfeeds becomes a real file with the content in use, before samples/ is
// replaced; with install and with bininstall.
func TestInstallConvertsConfigSymlinks(t *testing.T) {
	root := sourceRoot(t)
	for _, mode := range []string{"install", "bininstall"} {
		t.Run(mode, func(t *testing.T) {
			stage := t.TempDir()
			news := filepath.Join(stage, "news")
			install(t, root, "install", stage)
			old := []byte("# the old sample this site ran with\n")
			sample := filepath.Join(news, "samples", "dnewsfeeds")
			if err := os.Chmod(sample, 0o644); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(sample, old, 0o444); err != nil {
				t.Fatal(err)
			}
			cfg := filepath.Join(news, "dnewsfeeds")
			if err := os.Remove(cfg); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink("/news/samples/dnewsfeeds", cfg); err != nil {
				t.Fatal(err)
			}

			out := install(t, root, mode, stage)
			m := fileMode(t, cfg)
			if !m.IsRegular() || m.Perm() != 0o640 {
				t.Fatalf("dnewsfeeds after %s: mode %v, want a regular file 0640\n%s", mode, m, out)
			}
			if got, _ := os.ReadFile(cfg); !bytes.Equal(got, old) {
				t.Errorf("dnewsfeeds after %s: %q, want the content it had (the old sample)", mode, got)
			}
			want, _ := os.ReadFile(filepath.Join(root, "samples", "dnewsfeeds"))
			if got, _ := os.ReadFile(sample); !bytes.Equal(got, want) {
				t.Errorf("samples/dnewsfeeds not updated by %s", mode)
			}
		})
	}
}

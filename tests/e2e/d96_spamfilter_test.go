package e2e

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// TestSpamfilterClearNullHashAry: dspaminfo -B -c -1 must not crash when the
// spam cache file is inaccessible and HashAry is NULL. Fails on main:
// ClearSpamFilterEntry calls bzero(NULL, Hsize*sizeof(Filter)) -> SIGSEGV.
func TestSpamfilterClearNullHashAry(t *testing.T) {
	s := NewSite(t, SiteOptions{NoStart: true})

	// Block openFilterFile by planting a directory where the cache file would go.
	// open(path, O_RDWR|O_CREAT) on a directory returns EISDIR; HashAry stays NULL.
	if err := os.Mkdir(filepath.Join(s.Dir, "spam.body.cache"), 0o755); err != nil {
		t.Fatal(err)
	}

	out, err := s.TryRun("dspaminfo", "-B", "-c", "-1")
	t.Logf("dspaminfo: err=%v\n%s", err, out)

	var ee *exec.ExitError
	if errors.As(err, &ee) && ee.ExitCode() < 0 {
		t.Errorf("dspaminfo -B -c -1 was killed by signal (%v) when spam.body.cache is inaccessible", err)
	}
}

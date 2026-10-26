package e2e

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestDlockhistorySleepOption: dlockhistory -s takes its sleep time as an argument and keeps
// running. Before, "-s" was not declared to take one, and strtol(NULL) crashed it at once.
func TestDlockhistorySleepOption(t *testing.T) {
	s := NewSite(t, SiteOptions{NoStart: true})
	s.Run(t, "dhisbench", "-af", "1", "-ac", "1", "-f", filepath.Join(s.Dir, "dhistory"),
		"-m", filepath.Join(s.Dir, "bench.map"))
	cmd := exec.Command(filepath.Join(s.Bin, "dlockhistory"), "-f", "-s", "1", filepath.Join(s.Dir, "dhistory"))
	cmd.Env = s.Env
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		t.Fatalf("dlockhistory -f -s 1 exited at once: %v", err)
	case <-time.After(1500 * time.Millisecond):
		cmd.Process.Kill()
		<-done
	}
}

// TestDspaminfoClearNeedsCache: dspaminfo -c needs -B or -N (before, both caches were always
// selected and the check never fired; -c also set the debug level).
func TestDspaminfoClearNeedsCache(t *testing.T) {
	s := NewSite(t, SiteOptions{NoStart: true})
	out, _ := s.TryRun("dspaminfo", "-c", "0")
	if !strings.Contains(out, "Must specify which cache to clear") {
		t.Errorf("dspaminfo -c 0 without -B/-N: %q, want a refusal", out)
	}
}

// TestDcancelDryRunDoesNotRewriteSpool: -n reports the planned cancellation without entering
// the spool rewrite path or removing the article from the spool.
func TestDcancelDryRunDoesNotRewriteSpool(t *testing.T) {
	s := NewSite(t, SiteOptions{})
	id := s.Inject(t, "e2e.test", 1)[0]
	cmd := exec.Command(filepath.Join(s.Bin, "dcancel"), "-a", "-n")
	cmd.Dir = s.Dir
	cmd.Env = append(os.Environ(), s.Env...)
	cmd.Stdin = strings.NewReader(id + "\n")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("dcancel -a -n: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "Would cancel history entry for: "+id) {
		t.Errorf("dcancel -a -n did not report the planned cancellation:\n%s", out)
	}
	if strings.Contains(string(out), "Rewriting:") {
		t.Errorf("dcancel -a -n entered the spool rewrite path:\n%s", out)
	}
	p := s.Peer(t)
	expect(t, p, 223, "STAT %s", id)
	expect(t, p, 205, "QUIT")
}

// TestDcancelDryRunReaderDoesNotCancelOverview: -n applies to reader-index cancellation too.
func TestDcancelDryRunReaderDoesNotCancelOverview(t *testing.T) {
	st, spool := spoolStore(t)
	s := NewReaderSite(t, ReaderOptions{Spool: spool})
	id := msgID(t)
	feedReader(t, s, st, "e2e.test", newArticle(id, "e2e.test", "dry-run reader cancel", "body"))
	c := s.Reader(t)
	waitFor(t, 10*time.Second, "the article in the reader overview", func() bool {
		_, line := c.Cmd("GROUP e2e.test")
		f := strings.Fields(line)
		return len(f) > 1 && f[1] != "0"
	})
	expect(t, c, 223, "STAT 1")

	cmd := exec.Command(filepath.Join(s.Bin, "dcancel"), "-n", "-r")
	cmd.Dir = s.Dir
	cmd.Env = append(os.Environ(), s.Env...)
	cmd.Stdin = strings.NewReader("e2e.test:1\n")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("dcancel -n -r: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "Would cancel 1 in e2e.test") {
		t.Errorf("dcancel -n -r did not report the planned cancellation:\n%s", out)
	}
	expect(t, c, 223, "STAT 1")
	expect(t, c, 205, "QUIT")
}

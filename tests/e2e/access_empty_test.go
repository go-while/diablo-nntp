package e2e

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// Follow-ups to PR #160 (go-while). A dreader.access that is refused for a bad value on a
// fresh start (no older access cache) leaves the access map empty. Three things must hold:
// dreaderd must not crash when a client is resolved against that empty map, the parse error
// must be logged once per file change (not on every main-loop pass and connection), and a
// refused file fixed in place must still be picked up.
//
// These tests reuse helpers from readerdef_numbers_test.go (startReaderNoWait, readerGreeting,
// generalLog, accessWith) and site_test.go (waitFor, WriteFile, Site.Dir).

// TestAccessMapEmptyDeniesNoCrash (D-161 follow-up, point 4): with the access map empty,
// ReadAccessCache() loads nothing, so accessmap.ReaderList is NULL. The first client makes
// the main process run SetAuthDetails against it. On 157fc1d SetAuthDetails dereferences the
// NULL ReaderDef and the daemon crashes, so no connection is ever answered; with the fix the
// dr_*Def pointers fall back to zeroed objects, the client is denied, and dreaderd stays up.
func TestAccessMapEmptyDeniesNoCrash(t *testing.T) {
	s := NewReaderSite(t, ReaderOptions{SpoolAddr: "127.0.0.1:1", NoStart: true,
		Access: accessWith("maxconnperhost 1x"),
		Config: map[string]string{"path_generallog": "%s/general.log"}})
	startReaderNoWait(t, s)

	waitFor(t, 5*time.Second, "the bad-value line in the log", func() bool {
		return strings.Contains(generalLog(s), "bad value")
	})

	// The empty map is hit the moment a client connects. dreaderd must answer - with a
	// refusal - and keep serving; a crash leaves every connection unanswered.
	var g string
	waitFor(t, 10*time.Second, "dreaderd to answer a client against the empty access map", func() bool {
		g = readerGreeting(s.Addr)
		return g != ""
	})
	if strings.HasPrefix(g, "200") || strings.HasPrefix(g, "201") {
		t.Errorf("empty access map gave service: %q, want a refusal", strings.TrimSpace(g))
	}

	// Still alive: a second connection is answered too (the first did not take the daemon down).
	if readerGreeting(s.Addr) == "" {
		t.Fatalf("dreaderd stopped answering after a client hit the empty access map (NULL ReaderDef crash)")
	}
}

// TestAccessErrorLoggedOncePerChange (D-161 follow-up): InstallAccessCache used to set
// Access_LastUpdate = 0 on a parse error, so the "unchanged" check never matched and the file
// was re-parsed - and the error re-logged - on every main-loop pass (<=10s) and every new
// connection. The fix keeps the refused file's mtime/size/ino and skips it silently until it
// changes. The bad-value line must therefore appear exactly once while dreaderd runs and takes
// connections; once the file is fixed it is parsed again and service resumes.
func TestAccessErrorLoggedOncePerChange(t *testing.T) {
	s := NewReaderSite(t, ReaderOptions{SpoolAddr: "127.0.0.1:1", NoStart: true,
		Access: accessWith("maxconnperhost 1x"),
		Config: map[string]string{"path_generallog": "%s/general.log"}})

	// Age the refused file so its first parse treats it as settled and keeps its mtime: a file
	// written this same second is reparsed on the next pass (so a same-second in-place fix is
	// not missed - see TestAccessErrorFixedInPlaceSameSecond), which would log the error a
	// second time and is not what this test is about.
	old := time.Now().Add(-10 * time.Second)
	if err := os.Chtimes(filepath.Join(s.Dir, "dreader.access"), old, old); err != nil {
		t.Fatal(err)
	}

	startReaderNoWait(t, s)

	waitFor(t, 5*time.Second, "the bad-value line in the log", func() bool {
		return strings.Contains(generalLog(s), "bad value")
	})

	// Let the main loop take more than one idle pass (its timeout is 10s) over the unchanged
	// refused file. On 157fc1d every pass re-parses and logs the error again.
	time.Sleep(14 * time.Second)
	// New connections also make dreaderd look at dreader.access; they must not re-log either.
	for i := 0; i < 3; i++ {
		readerGreeting(s.Addr)
	}
	if n := strings.Count(generalLog(s), "bad value"); n != 1 {
		t.Fatalf("the bad-value line appears %d times, want once (file re-parsed on every pass/connection)", n)
	}

	// Fixing the file changes its mtime/size, so it is parsed again and now installs:
	// service resumes. Each poll is itself a connection, which triggers the re-parse.
	s.WriteFile(t, "dreader.access", DefaultReaderAccess)
	var g string
	waitFor(t, 20*time.Second, "service to resume after dreader.access is fixed", func() bool {
		g = readerGreeting(s.Addr)
		return strings.HasPrefix(g, "200") || strings.HasPrefix(g, "201")
	})
}

// TestAccessErrorFixedInPlaceSameSecond (D-161 follow-up): keeping a refused file's
// mtime/size/ino opens a race. A bad file that is fixed in place within the same second with a
// value of the same length has an unchanged mtime, size and inode, so the "unchanged" check
// would skip the fix forever and the reader would give no service until the next edit. The
// guard keeps the triple only when the file was already older than the moment dreaderd read it,
// so a file that is not settled is parsed again.
//
// The race is reproduced deterministically with a future mtime: a file stamped in the future is
// never "settled", so with the guard it is always reparsed and the fix installs, independent of
// how fast this test runs. On 2a03bd3 the triple is kept unconditionally, so the unchanged file
// identity (same future mtime, same size, same inode after an in-place same-length rewrite) is
// matched and the fix is never seen, and this test times out.
func TestAccessErrorFixedInPlaceSameSecond(t *testing.T) {
	s := NewReaderSite(t, ReaderOptions{SpoolAddr: "127.0.0.1:1", NoStart: true,
		Access: accessWith("maxconnperhost 1x"),
		Config: map[string]string{"path_generallog": "%s/general.log"}})

	// A future mtime is never settled, so the guard always reparses the file - the race is then
	// purely about the mtime/size/ino triple being unchanged, not about timing.
	accessPath := filepath.Join(s.Dir, "dreader.access")
	future := time.Now().Add(60 * time.Second)
	if err := os.Chtimes(accessPath, future, future); err != nil {
		t.Fatal(err)
	}

	startReaderNoWait(t, s)

	waitFor(t, 5*time.Second, "the bad-value line in the log", func() bool {
		return strings.Contains(generalLog(s), "bad value")
	})

	// Rewrite in place with a valid value of the same length ("1x" -> "10") and restore the same
	// future mtime, so the mtime/size/ino triple is exactly what the refused parse recorded.
	// os.WriteFile briefly leaves the mtime at "now"; freeze the main dreaderd across the rewrite
	// so no main-loop pass or connection can observe that transient mtime and install the fix on
	// its own (which would let 2a03bd3 pass by accident). On 2a03bd3 the restored triple matches
	// the kept record and the fix is skipped; with the guard the never-settled file left
	// Access_LastUpdate zeroed, so it is parsed again and the valid config installs.
	pid := readMainPid(t, s)
	proc, err := os.FindProcess(pid)
	if err != nil {
		t.Fatal(err)
	}
	if err := proc.Signal(syscall.SIGSTOP); err != nil {
		t.Fatalf("SIGSTOP dreaderd %d: %v", pid, err)
	}
	defer proc.Signal(syscall.SIGCONT) // safety net if an assertion below stops the test
	if err := os.WriteFile(accessPath, []byte(accessWith("maxconnperhost 10")), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(accessPath, future, future); err != nil {
		t.Fatal(err)
	}
	if err := proc.Signal(syscall.SIGCONT); err != nil {
		t.Fatalf("SIGCONT dreaderd %d: %v", pid, err)
	}

	var g string
	waitFor(t, 20*time.Second, "service to resume after a same-second in-place fix", func() bool {
		g = readerGreeting(s.Addr)
		return strings.HasPrefix(g, "200") || strings.HasPrefix(g, "201")
	})
}

// readMainPid returns the main dreaderd process id from the site's pid file (log/dreaderd.pid,
// written by main.c at startup), waiting briefly for dreaderd to write it.
func readMainPid(t *testing.T, s *Site) int {
	t.Helper()
	var pid int
	waitFor(t, 5*time.Second, "dreaderd.pid", func() bool {
		b, err := os.ReadFile(filepath.Join(s.Dir, "log", "dreaderd.pid"))
		if err != nil {
			return false
		}
		n, err := strconv.Atoi(strings.TrimSpace(string(b)))
		if err != nil || n <= 0 {
			return false
		}
		pid = n
		return true
	})
	return pid
}

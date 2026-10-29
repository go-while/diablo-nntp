package e2e

import (
	"strings"
	"testing"
	"time"
)

// Follow-ups to PR #160 (go-while). A dreader.access that is refused for a bad value on a
// fresh start (no older access cache) leaves the access map empty. Two things must hold:
// dreaderd must not crash when a client is resolved against that empty map, and the parse
// error must be logged once per file change, not on every main-loop pass and connection.
//
// Both tests reuse helpers from readerdef_numbers_test.go (startReaderNoWait, readerGreeting,
// generalLog, accessWith) and site_test.go (waitFor, WriteFile).

// TestAccessMapEmptyDeniesNoCrash (D-160 follow-up, point 4): with the access map empty,
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

// TestAccessErrorLoggedOncePerChange (D-160 follow-up): InstallAccessCache used to set
// Access_LastUpdate = 0 on a parse error, so the "unchanged" check never matched and the file
// was re-parsed - and the error re-logged - on every main-loop pass (<=10s) and every new
// connection. The fix keeps the refused file's mtime/size/ino and skips it silently until it
// changes. The bad-value line must therefore appear exactly once while dreaderd runs and takes
// connections; once the file is fixed it is parsed again and service resumes.
func TestAccessErrorLoggedOncePerChange(t *testing.T) {
	s := NewReaderSite(t, ReaderOptions{SpoolAddr: "127.0.0.1:1", NoStart: true,
		Access: accessWith("maxconnperhost 1x"),
		Config: map[string]string{"path_generallog": "%s/general.log"}})
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

package e2e

import (
	"strings"
	"testing"
	"time"
)

// Follow-ups to PR #160 (go-while). A dreader.access that is refused for a bad value on a
// fresh start (no older access cache) leaves the access map empty. dreaderd must not crash
// when a client is resolved against that empty map.
//
// This test reuses helpers from readerdef_numbers_test.go (startReaderNoWait, readerGreeting,
// generalLog, accessWith) and site_test.go (waitFor).

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

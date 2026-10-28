package e2e

// Cancel-Lock (RFC 8315): dreaderd checks a cancel's or supersede's Cancel-Key against the
// target's Cancel-Lock, depending on diablo.config readercanlock.

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/base64"
	"hash"
	"os"
	"path/filepath"
	"strings"
	"testing"

	testkit "github.com/go-while/nntp-testkit"
	"github.com/go-while/nntp-testkit/client"
	"github.com/go-while/nntp-testkit/fakeserver"
)

// canKey returns the Cancel-Key value base64(HMAC(secret, uid+mid)) (RFC 8315 section 4).
func canKey(h func() hash.Hash, secret, uid, mid string) string {
	m := hmac.New(h, []byte(secret))
	m.Write([]byte(uid + mid))
	return base64.StdEncoding.EncodeToString(m.Sum(nil))
}

// canLock returns the Cancel-Lock value for a Cancel-Key value: base64(H(key)).
func canLock(h func() hash.Hash, key string) string {
	d := h()
	d.Write([]byte(key))
	return base64.StdEncoding.EncodeToString(d.Sum(nil))
}

// TestCanLockVectors pins canKey and canLock to INN's test vectors (tests/lib/canlock-t.c),
// so the e2e tests compute what INN computes.
func TestCanLockVectors(t *testing.T) {
	const mid, user, admin, usec = "<12345@mid.example>", "JaneDoe", "ExampleSecret", "AnotherSecret"
	for _, c := range []struct {
		name, got, want string
	}{
		{"sha1 admin key", canKey(sha1.New, admin, "", mid), "8HzrY7F4N+5SXkGQah1mcyW+01g="},
		{"sha256 admin key", canKey(sha256.New, admin, "", mid), "qv1VXHYiCGjkX/N1nhfYKcAeUn8bCVhrWhoKuBSnpMA="},
		{"sha1 user key", canKey(sha1.New, usec, user, mid), "mHkFFHF4DM97Oi+8vEsCXQb4aM0="},
		{"sha256 user key", canKey(sha256.New, usec, user, mid), "yM0ep490Fzt83CLYYAytm3S2HasHhYG4LAeAlmuSEys="},
		{"sha1 admin lock", canLock(sha1.New, canKey(sha1.New, admin, "", mid)), "JD+QmQh5LH6lLLToKLcDl+Aemg0="},
		{"sha256 admin lock", canLock(sha256.New, canKey(sha256.New, admin, "", mid)), "s/pmK/3grrz++29ce2/mQydzJuc7iqHn1nqcJiQTPMc="},
		{"sha1 user lock", canLock(sha1.New, canKey(sha1.New, usec, user, mid)), "zdoRY4lJw5jLLtJTfpEqu1epqzc="},
		{"sha256 user lock", canLock(sha256.New, canKey(sha256.New, usec, user, mid)), "NSBTz7BfcQFTCen+U4lQ0VS8VIlZao2b8mxD/xJaaeE="},
	} {
		if c.got != c.want {
			t.Errorf("%s: %s, want %s (INN tests/lib/canlock-t.c)", c.name, c.got, c.want)
		}
	}
}

// skipWithoutCanLock skips the test unless the dreaderd in DIABLO_BIN knows readercanlock.
// It must run before a site with readercanlock in diablo.config is written: without
// Cancel-Lock support, dgrpctl and dreaderd reject the unknown directive.
func skipWithoutCanLock(tb testing.TB) {
	tb.Helper()
	b, err := os.ReadFile(filepath.Join(bin(tb), "dreaderd"))
	if err != nil || !bytes.Contains(b, []byte("readercanlock")) {
		tb.Skip("dreaderd has no readercanlock (Cancel-Lock support)")
	}
	// the symbol name is in dreaderd only when it is built with CANLOCK_ENABLED (libcanlock)
	if !bytes.Contains(b, []byte("cl_verify_multi")) {
		tb.Skip("dreaderd is built without libcanlock (CANLOCK_ENABLED)")
	}
}

// withHeaders inserts header lines before the blank line of an article.
func withHeaders(art []string, hdrs ...string) []string {
	head, body := splitArticle(art)
	out := append(append(append([]string{}, head...), hdrs...), "")
	return append(out, body...)
}

// cancelArticle returns a cancel control article for target in group, with the given extra
// headers (Cancel-Key).
func cancelArticle(tb testing.TB, target, group string, hdrs ...string) []string {
	return withHeaders(newArticle(msgID(tb), group, "cmsg cancel "+target, "cancel"),
		append([]string{"Control: cancel " + target}, hdrs...)...)
}

// canLockSite starts a reader site with readercanlock mode and a dcontrol.ctl that executes
// cancels, or skips the test when dreaderd has no Cancel-Lock support.
func canLockSite(tb testing.TB, mode string) (*Site, *fakeserver.Store) {
	tb.Helper()
	skipWithoutCanLock(tb)
	return canLockSiteAny(tb, mode)
}

// canLockSiteAny is canLockSite for any build; mode "" leaves readercanlock out of diablo.config
// (the default).
func canLockSiteAny(tb testing.TB, mode string) (*Site, *fakeserver.Store) {
	tb.Helper()
	st, spool := spoolStore(tb)
	cfg := map[string]string{}
	if mode != "" {
		cfg["readercanlock"] = mode
	}
	s := NewReaderSite(tb, ReaderOptions{
		Spool:   spool,
		Config:  cfg,
		Files:   map[string]string{"dcontrol.ctl": "cancel:*:*:doit,break\n"},
		NoStart: true,
	})
	_, port, _ := strings.Cut(s.Addr, ":")
	startReader(tb, s, "[127.0.0.1]:"+port)
	return s, st
}

// feedAndSettle feeds arts, then a marker article, and waits until the marker is in the
// group: dreaderd processes a feed in order, so every cancel before it has been handled.
func feedAndSettle(tb testing.TB, s *Site, st *fakeserver.Store, c *client.Conn, arts ...[]string) {
	tb.Helper()
	marker := msgID(tb)
	feedReader(tb, s, st, "e2e.test", append(arts, newArticle(marker, "e2e.test", "marker", "m"))...)
	waitFor(tb, testkit.Deadline(), "the marker article", func() bool {
		code, _ := c.Cmd("STAT %s", marker)
		if code == 223 {
			return true
		}
		_, _ = c.Cmd("GROUP e2e.test") // refresh the group view
		return false
	})
}

// present reports whether article number n of e2e.test is still there (STAT n).
func present(tb testing.TB, c *client.Conn, n int) bool {
	tb.Helper()
	expect(tb, c, 211, "GROUP e2e.test")
	code, _ := c.Cmd("STAT %d", n)
	return code == 223
}

const adminSecret = "e2e-canlock-secret"

func TestCancelLock(t *testing.T) {
	type tc struct {
		name    string
		mode    string
		lock    func(mid string) string // Cancel-Lock header value of the target; nil = none
		key     func(mid string) string // Cancel-Key header value of the cancel; nil = none
		removed bool
	}
	sha256Lock := func(mid string) string {
		return "sha256:" + canLock(sha256.New, canKey(sha256.New, adminSecret, "", mid))
	}
	sha256Key := func(mid string) string { return "sha256:" + canKey(sha256.New, adminSecret, "", mid) }
	foldedLock := func(mid string) string {
		return "sha1:" + canLock(sha1.New, canKey(sha1.New, adminSecret, "", mid)) +
			" (a comment)\r\n\tsha256:" + canLock(sha256.New, canKey(sha256.New, "other-secret", "", mid))
	}
	sha1Key := func(mid string) string { return "sha1:" + canKey(sha1.New, adminSecret, "", mid) }
	wrongKey := func(mid string) string { return "sha256:" + canKey(sha256.New, "wrong-secret", "", mid) }
	for _, c := range []tc{
		{"require-auth/locked/matching key", "require-auth", sha256Lock, sha256Key, true},
		{"require-auth/locked/wrong key", "require-auth", sha256Lock, wrongKey, false},
		{"require-auth/locked/no key", "require-auth", sha256Lock, nil, false},
		{"require-auth/unlocked", "require-auth", nil, sha256Key, false},
		{"auth/locked/wrong key", "auth", sha256Lock, wrongKey, false},
		{"auth/unlocked/no key", "auth", nil, nil, true},
		{"all/unlocked/no key", "all", nil, nil, true},
		{"none/locked/matching key", "none", sha256Lock, sha256Key, false},
		{"require-auth/folded lock/sha1 key", "require-auth", foldedLock, sha1Key, true},
	} {
		t.Run(c.name, func(t *testing.T) {
			s, st := canLockSite(t, c.mode)
			r := s.Reader(t)
			target := msgID(t)
			art := newArticle(target, "e2e.test", "target", "body")
			if c.lock != nil {
				art = withHeaders(art, "Cancel-Lock: "+c.lock(target))
			}
			var hdrs []string
			if c.key != nil {
				hdrs = append(hdrs, "Cancel-Key: "+c.key(target))
			}
			feedAndSettle(t, s, st, r, art)
			if !present(t, r, 1) {
				t.Fatalf("target is not article 1 of e2e.test")
			}
			feedAndSettle(t, s, st, r, cancelArticle(t, target, "e2e.test", hdrs...))
			if got := !present(t, r, 1); got != c.removed {
				t.Errorf("target cancelled: %v, want %v (readercanlock %s)", got, c.removed, c.mode)
			}
		})
	}
}

// TestCancelLockSupersedes: Supersedes goes through the same check as a cancel.
func TestCancelLockSupersedes(t *testing.T) {
	for _, good := range []bool{true, false} {
		name := map[bool]string{true: "matching key", false: "wrong key"}[good]
		t.Run(name, func(t *testing.T) {
			s, st := canLockSite(t, "require-auth")
			r := s.Reader(t)
			old := msgID(t)
			feedAndSettle(t, s, st, r, withHeaders(newArticle(old, "e2e.test", "v1", "old"),
				"Cancel-Lock: sha256:"+canLock(sha256.New, canKey(sha256.New, adminSecret, "", old))))
			secret := adminSecret
			if !good {
				secret = "wrong-secret"
			}
			newer := withHeaders(newArticle(msgID(t), "e2e.test", "v2", "new"),
				"Supersedes: "+old, "Cancel-Key: sha256:"+canKey(sha256.New, secret, "", old))
			feedAndSettle(t, s, st, r, newer)
			if gone := !present(t, r, 1); gone != good {
				t.Errorf("superseded article gone: %v, want %v", gone, good)
			}
		})
	}
}

// TestCancelLockEarlyCancel: a cancel that arrives before its article can't be checked;
// under require-auth it is dropped (the article is accepted later), under all it goes into
// the pre-cancel cache and the article is refused with 437, as before.
func TestCancelLockEarlyCancel(t *testing.T) {
	for _, c := range []struct {
		mode string
		want int
	}{{"require-auth", 235}, {"all", 437}} {
		t.Run(c.mode, func(t *testing.T) {
			s, st := canLockSite(t, c.mode)
			r := s.Reader(t)
			target := msgID(t)
			feedAndSettle(t, s, st, r, cancelArticle(t, target, "e2e.test"))
			p := s.dialFrom(t, PeerIP)
			expect(t, p, 335, "IHAVE %s", target)
			send(t, p, wire(newArticle(target, "e2e.test", "late", "body")))
			if got := status(t, p, c.want, "IHAVE of the cancelled article"); got == "" {
				t.Fatal("no status")
			}
			expect(t, p, 205, "QUIT")
		})
	}
}

// TestCancelLockTrackonlyRefused: readercanlock (other than all) with readerxover trackonly
// can't check anything, so dreaderd refuses to start.
func TestCancelLockTrackonlyRefused(t *testing.T) {
	skipWithoutCanLock(t)
	_, spool := spoolStore(t)
	s := NewReaderSite(t, ReaderOptions{
		Spool:   spool,
		Config:  map[string]string{"readerxover": "trackonly"},
		NoStart: true,
	})
	_, port, _ := strings.Cut(s.Addr, ":")
	out, err := s.TryRun("dreaderd", "-d", "-p", PathHost, "-h", PathHost, "-P", "[127.0.0.1]:"+port)
	if err == nil || !strings.Contains(out, "trackonly") {
		t.Errorf("dreaderd with trackonly and the default readercanlock: err %v, output %q; want it to refuse", err, out)
	}
}

// skipWithCanLock skips the test when dreaderd is built with libcanlock: the tests that use it
// check what a build without the library does.
func skipWithCanLock(tb testing.TB) {
	tb.Helper()
	b, err := os.ReadFile(filepath.Join(bin(tb), "dreaderd"))
	if err != nil || !bytes.Contains(b, []byte("readercanlock")) {
		tb.Skip("dreaderd has no readercanlock (Cancel-Lock support)")
	}
	if bytes.Contains(b, []byte("cl_verify_multi")) {
		tb.Skip("dreaderd is built with libcanlock")
	}
}

// TestCancelLockWithoutLibcanlock: in a build without libcanlock no Cancel-Key can be verified,
// and the safe modes stay safe: the default (require-auth), require-auth and none execute no
// cancel (not even one with a matching key), auth withdraws only an article without a
// Cancel-Lock, and all executes everything, as before readercanlock existed. Every mode is
// accepted in diablo.config.
func TestCancelLockWithoutLibcanlock(t *testing.T) {
	skipWithCanLock(t)
	sha256Lock := func(mid string) string {
		return "sha256:" + canLock(sha256.New, canKey(sha256.New, adminSecret, "", mid))
	}
	sha256Key := func(mid string) string { return "sha256:" + canKey(sha256.New, adminSecret, "", mid) }
	for _, c := range []struct {
		name, mode string
		locked     bool
		removed    bool
	}{
		{"default/locked/matching key", "", true, false},
		{"default/unlocked", "", false, false},
		{"require-auth/locked/matching key", "require-auth", true, false},
		{"auth/locked/matching key", "auth", true, false},
		{"auth/unlocked", "auth", false, true},
		{"none/unlocked", "none", false, false},
		{"all/locked/matching key", "all", true, true},
	} {
		t.Run(c.name, func(t *testing.T) {
			s, st := canLockSiteAny(t, c.mode)
			r := s.Reader(t)
			target := msgID(t)
			art := newArticle(target, "e2e.test", "target", "body")
			if c.locked {
				art = withHeaders(art, "Cancel-Lock: "+sha256Lock(target))
			}
			feedAndSettle(t, s, st, r, art)
			if !present(t, r, 1) {
				t.Fatalf("target is not article 1 of e2e.test")
			}
			feedAndSettle(t, s, st, r, cancelArticle(t, target, "e2e.test", "Cancel-Key: "+sha256Key(target)))
			if got := !present(t, r, 1); got != c.removed {
				t.Errorf("target cancelled: %v, want %v (readercanlock %q)", got, c.removed, c.mode)
			}
		})
	}
}

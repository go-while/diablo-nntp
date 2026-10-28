package e2e

// dcanlock: prints the Cancel-Key (the admin key, like INN's gencancel -k) and the Cancel-Lock
// for a Message-ID from the canlockadmin and canlockuser secrets of the vserverdef, and checks
// a key against a lock. Go computes the expected values (canKey and canLock, pinned to INN's
// vectors in canlock_test.go).

import (
	"bytes"
	"crypto/sha1"
	"crypto/sha256"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// skipWithoutDcanlock skips the test when DIABLO_BIN has no dcanlock.
func skipWithoutDcanlock(tb testing.TB) {
	tb.Helper()
	if _, err := os.Stat(filepath.Join(bin(tb), "dcanlock")); err != nil {
		tb.Skip("no dcanlock in DIABLO_BIN")
	}
}

// dcanlockSite writes a reader site (not started) whose dreader.access has vs in the vserverdef.
func dcanlockSite(tb testing.TB, vs string) *Site {
	tb.Helper()
	skipWithoutDcanlock(tb)
	_, spool := spoolStore(tb)
	return NewReaderSite(tb, ReaderOptions{Spool: spool, Access: genAccess(true, vs, ""), NoStart: true})
}

// dcanlock runs dcanlock with stdin and returns its output and exit status.
func dcanlock(tb testing.TB, s *Site, stdin string, args ...string) (string, int) {
	tb.Helper()
	cmd := exec.Command(filepath.Join(s.Bin, "dcanlock"), args...)
	cmd.Dir = s.Dir
	cmd.Env = append(os.Environ(), s.Env...)
	cmd.Stdin = strings.NewReader(stdin)
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	err := cmd.Run()
	var ee *exec.ExitError
	switch {
	case err == nil:
		return strings.TrimSpace(out.String()), 0
	case errors.As(err, &ee):
		return strings.TrimSpace(out.String()), ee.ExitCode()
	}
	tb.Fatalf("dcanlock %v: %v", args, err)
	return "", -1
}

// TestDcanlock: key and lock for the admin and the user secrets, the schemes, several
// Message-IDs, and a secret from standard input.
func TestDcanlock(t *testing.T) {
	two := vsBoth + "    canlockadmin " + genOldSecret + "\n"
	s := dcanlockSite(t, two)
	mid, mid2 := msgID(t), msgID(t)
	for _, c := range []struct {
		name string
		args []string
		want []string
	}{
		{"admin keys, every secret", []string{"key", mid},
			sorted(wantKeys(genAdminSecret, "", mid, true), wantKeys(genOldSecret, "", mid, true))},
		{"admin locks, every secret", []string{"lock", mid},
			sorted(wantLocks(genAdminSecret, "", mid, true), wantLocks(genOldSecret, "", mid, true))},
		{"user keys", []string{"-u", "fred", "key", mid}, sorted(wantKeys(genUserSecret, "fred", mid, true))},
		{"user locks", []string{"-u", "fred", "lock", mid}, sorted(wantLocks(genUserSecret, "fred", mid, true))},
		{"sha1 only", []string{"-s", "sha1", "key", mid},
			sorted([]string{"sha1:" + canKey(sha1.New, genAdminSecret, "", mid), "sha1:" + canKey(sha1.New, genOldSecret, "", mid)})},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, code := dcanlock(t, s, "", c.args...)
			if code != 0 {
				t.Fatalf("exit %d: %s", code, out)
			}
			if got := elements(out); !slices.Equal(got, c.want) {
				t.Errorf("elements %q, want %q", got, c.want)
			}
		})
	}

	t.Run("one line for each Message-ID", func(t *testing.T) {
		out, code := dcanlock(t, s, "", "-u", "fred", "key", mid, mid2)
		lines := strings.Split(out, "\n")
		if code != 0 || len(lines) != 2 {
			t.Fatalf("exit %d, %d lines: %q", code, len(lines), out)
		}
		for i, m := range []string{mid, mid2} {
			if got := elements(lines[i]); !slices.Equal(got, sorted(wantKeys(genUserSecret, "fred", m, true))) {
				t.Errorf("line %d: %q", i+1, lines[i])
			}
		}
	})

	t.Run("canlocksha1 no", func(t *testing.T) {
		s := dcanlockSite(t, vsNoSh)
		out, _ := dcanlock(t, s, "", "key", mid)
		if want := []string{"sha256:" + canKey(sha256.New, genAdminSecret, "", mid)}; !slices.Equal(elements(out), want) {
			t.Errorf("elements %q, want %q", elements(out), want)
		}
	})

	t.Run("secret from standard input", func(t *testing.T) {
		out, code := dcanlock(t, s, "stdin-secret\n", "-S", "key", mid)
		want := sorted(wantKeys("stdin-secret", "", mid, true))
		if code != 0 || !slices.Equal(elements(out), want) {
			t.Errorf("exit %d, elements %q, want %q", code, elements(out), want)
		}
	})

	// -S and verify work on a machine that has no diablo.config and no dreader.access
	t.Run("no configuration needed", func(t *testing.T) {
		run := func(stdin string, args ...string) (string, int) {
			cmd := exec.Command(filepath.Join(s.Bin, "dcanlock"), args...)
			cmd.Dir = t.TempDir()
			for _, e := range os.Environ() {
				if !strings.HasPrefix(e, "DIABLO_CONFIG_PATH=") {
					cmd.Env = append(cmd.Env, e)
				}
			}
			cmd.Stdin = strings.NewReader(stdin)
			out, err := cmd.CombinedOutput()
			code := 0
			var ee *exec.ExitError
			if errors.As(err, &ee) {
				code = ee.ExitCode()
			} else if err != nil {
				t.Fatal(err)
			}
			return strings.TrimSpace(string(out)), code
		}
		key, code := run("stdin-secret\n", "-S", "key", mid)
		if want := sorted(wantKeys("stdin-secret", "", mid, true)); code != 0 || !slices.Equal(elements(key), want) {
			t.Errorf("-S key: exit %d, elements %q, want %q", code, elements(key), want)
		}
		lock, code := run("stdin-secret\n", "-S", "lock", mid)
		if code != 0 {
			t.Fatalf("-S lock: exit %d: %s", code, lock)
		}
		if out, code := run("", "verify", key, lock); code != 0 {
			t.Errorf("verify: exit %d: %s", code, out)
		}
	})
}

// TestDcanlockVerify: a key made by dcanlock matches the lock made by dcanlock (and a lock
// made by dreaderd's formula), a key of another message or secret doesn't.
func TestDcanlockVerify(t *testing.T) {
	s := dcanlockSite(t, vsBoth)
	mid := msgID(t)
	key, _ := dcanlock(t, s, "", "key", mid)
	lock, _ := dcanlock(t, s, "", "lock", mid)
	otherKey, _ := dcanlock(t, s, "", "key", msgID(t))
	userKey, _ := dcanlock(t, s, "", "-u", "fred", "key", mid)
	for _, c := range []struct {
		name, key, lock string
		code            int
	}{
		{"matching", key, lock, 0},
		{"sha1 key, folded lock with a comment", strings.Fields(key)[1],
			"sha256:xx (a comment)\n\t" + lock, 0},
		{"key of another message", otherKey, lock, 1},
		{"user key against the admin lock", userKey, lock, 1},
		{"no key", "", lock, 1},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, code := dcanlock(t, s, "", "verify", c.key, c.lock)
			if code != c.code {
				t.Errorf("exit %d (%s), want %d", code, out, c.code)
			}
		})
	}
}

// TestDcanlockErrors: what the tool refuses, with status 2.
func TestDcanlockErrors(t *testing.T) {
	s := dcanlockSite(t, "    canlockadmin "+genAdminSecret+"\n")
	for _, c := range []struct {
		name string
		args []string
		want string
	}{
		{"not a Message-ID", []string{"key", "no-brackets@example"}, "not a Message-ID"},
		{"Message-ID with a space", []string{"key", "<a b@example>"}, "not a Message-ID"},
		{"no user secret", []string{"-u", "fred", "key", "<a@example>"}, "no canlockuser secret"},
		{"no such vserverdef", []string{"-v", "nosuch", "key", "<a@example>"}, "no vserverdef nosuch"},
		{"bad scheme", []string{"-s", "md5", "key", "<a@example>"}, "sha256, sha1 or sha512"},
		{"no Message-ID", []string{"key"}, "Usage"},
		{"unknown command", []string{"frobnicate"}, "Usage"},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, code := dcanlock(t, s, "", c.args...)
			if code != 2 || !strings.Contains(out, c.want) {
				t.Errorf("exit %d, output %q; want 2 and %q", code, out, c.want)
			}
		})
	}
}

// TestDcanlockWithdraw: the use case. An article posted through dreaderd (with its locks) is
// withdrawn by a cancel that carries the admin key dcanlock prints, by a reader in
// require-auth mode.
func TestDcanlockWithdraw(t *testing.T) {
	skipWithoutDcanlock(t)
	g := newGenSite(t, true, vsBoth, "", "require-auth")
	mid := msgID(t)
	g.post(t, g.poster(t), "Message-ID: "+mid)
	r := g.Reader(t)
	expect(t, r, 381, "AUTHINFO USER fred")
	expect(t, r, 281, "AUTHINFO PASS fredspass")
	feedAndSettle(t, g.Site, g.st, r, fromElsewhere(g.last()))
	if !present(t, r, 1) {
		t.Fatal("the posted article is not article 1 of e2e.test")
	}
	key, code := dcanlock(t, g.Site, "", "key", mid)
	if code != 0 {
		t.Fatalf("dcanlock key: exit %d: %s", code, key)
	}
	feedAndSettle(t, g.Site, g.st, r, cancelArticle(t, mid, "e2e.test", "Cancel-Key: "+key))
	if present(t, r, 1) {
		t.Error("the article is still there after the admin cancel")
	}
}

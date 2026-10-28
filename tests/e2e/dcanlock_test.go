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
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/go-while/nntp-testkit/client"
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
	return NewReaderSite(tb, ReaderOptions{Spool: spool, Access: genAccess(true, vs, ""), NoStart: true,
		Config: map[string]string{"newsadmin": "admin@example.invalid", "readerhostname": "news.example.invalid"}})
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
		{"sha1 only", []string{"-t", "sha1", "key", mid},
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

	t.Run("secret from standard input, canlocksha1 of -A", func(t *testing.T) {
		acc := filepath.Join(t.TempDir(), "dreader.access")
		if err := os.WriteFile(acc, []byte(genAccess(true, vsNoSh, "")), 0o600); err != nil {
			t.Fatal(err)
		}
		out, code := dcanlock(t, s, "stdin-secret\n", "-S", "-A", acc, "key", mid)
		want := []string{"sha256:" + canKey(sha256.New, "stdin-secret", "", mid)}
		if code != 0 || !slices.Equal(elements(out), want) {
			t.Errorf("exit %d, elements %q, want %q (sha1 left out by canlocksha1 no)", code, elements(out), want)
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
		{"bad scheme", []string{"-t", "md5", "key", "<a@example>"}, "sha256, sha1 or sha512"},
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

// splitCancel splits the article dcanlock cancel writes into its header lines and its body.
func splitCancel(t *testing.T, out string) (head []string, body string) {
	t.Helper()
	// dcanlock writes CR LF line ends, as they go over NNTP; every line must have both
	if strings.Count(out, "\n") != strings.Count(out, "\r\n") {
		t.Errorf("the cancel article has line ends without CR: %q", out)
	}
	out = strings.ReplaceAll(out, "\r\n", "\n")
	h, b, ok := strings.Cut(out, "\n\n")
	if !ok {
		t.Fatalf("no blank line between the headers and the body: %q", out)
	}
	return strings.Split(h, "\n"), b
}

// TestDcanlockCancel: the whole cancel article, with its options.
func TestDcanlockCancel(t *testing.T) {
	s := dcanlockSite(t, vsBoth+"    canlockadmin "+genOldSecret+"\n")
	mid := msgID(t)
	out, code := dcanlock(t, s, "", "-n", "e2e.test,e2e.other", "cancel", mid)
	if code != 0 {
		t.Fatalf("exit %d: %s", code, out)
	}
	head, body := splitCancel(t, out)
	for name, want := range map[string]string{
		"From":                      "admin@example.invalid",
		"Newsgroups":                "e2e.test,e2e.other",
		"Subject":                   "cmsg cancel " + mid,
		"Approved":                  "admin@example.invalid",
		"Control":                   "cancel " + mid,
		"MIME-Version":              "1.0",
		"Content-Type":              "text/plain; charset=ISO-8859-1",
		"Content-Transfer-Encoding": "8bit",
	} {
		if got := headerValue(head, name); got != want {
			t.Errorf("%s: %q, want %q", name, got, want)
		}
	}
	if want := sorted(wantKeys(genAdminSecret, "", mid, true), wantKeys(genOldSecret, "", mid, true)); !slices.Equal(elements(headerValue(head, "Cancel-Key")), want) {
		t.Errorf("Cancel-Key %q, want %q", headerValue(head, "Cancel-Key"), want)
	}
	if id := headerValue(head, "Message-ID"); !strings.HasPrefix(id, "<cancel.") || !strings.HasSuffix(id, "@news.example.invalid>") || !validMsgIDText(id) {
		t.Errorf("Message-ID %q", id)
	}
	if d := headerValue(head, "Date"); !strings.Contains(d, ",") || len(d) < 25 {
		t.Errorf("Date %q", d)
	}
	if body != "Admin cancel." { // the helper trims the last newline
		t.Errorf("body %q", body)
	}

	// two cancels get different Message-IDs
	out2, _ := dcanlock(t, s, "", "-n", "e2e.test", "cancel", mid)
	head2, _ := splitCancel(t, out2)
	if headerValue(head2, "Message-ID") == headerValue(head, "Message-ID") {
		t.Error("two cancels have the same Message-ID")
	}

	t.Run("options", func(t *testing.T) {
		out, code := dcanlock(t, s, "", "-n", "e2e.test", "-f", "Boss <boss@example.invalid>", "-s", "Withdrawn",
			"-b", "Spam.", "-c", "UTF-8", "-d", "-m", "-a", "cancel", mid)
		if code != 0 {
			t.Fatalf("exit %d: %s", code, out)
		}
		head, body := splitCancel(t, out)
		if headerValue(head, "From") != "Boss <boss@example.invalid>" || headerValue(head, "Subject") != "Withdrawn" ||
			headerValue(head, "Content-Type") != "text/plain; charset=UTF-8" || body != "Spam." {
			t.Errorf("headers %q body %q", head, body)
		}
		if !strings.HasSuffix(headerValue(head, "Date"), "+0000") {
			t.Errorf("Date %q is not UTC", headerValue(head, "Date"))
		}
		for _, no := range []string{"Message-ID", "Approved"} {
			if headerCount(head, no) != 0 {
				t.Errorf("%s present with the option that leaves it out", no)
			}
		}
	})

	t.Run("no key, no secret needed", func(t *testing.T) {
		out, code := dcanlock(t, s, "", "-L", "-n", "e2e.test", "-A", "/nonexistent", "cancel", mid)
		head, _ := splitCancel(t, out)
		if code != 0 || headerCount(head, "Cancel-Key") != 0 {
			t.Errorf("exit %d, Cancel-Key count %d", code, headerCount(head, "Cancel-Key"))
		}
	})

	t.Run("refusals", func(t *testing.T) {
		for _, c := range []struct {
			name string
			args []string
			want string
		}{
			{"no Newsgroups and no article", []string{"cancel", mid}, "give them with -n"},
			{"bad Newsgroups", []string{"-n", "a b", "cancel", mid}, "bad Newsgroups"},
			{"non-ASCII subject", []string{"-n", "e2e.test", "-s", "caf\xe9", "cancel", mid}, "plain ASCII"},
			{"two Message-IDs", []string{"-n", "e2e.test", "cancel", mid, mid}, "Usage"},
			{"not a Message-ID", []string{"-n", "e2e.test", "cancel", "nonsense"}, "not a Message-ID"},
		} {
			out, code := dcanlock(t, s, "", c.args...)
			if code != 2 || !strings.Contains(out, c.want) {
				t.Errorf("%s: exit %d, output %q; want 2 and %q", c.name, code, out, c.want)
			}
		}
	})
}

// validMsgIDText: a Message-ID with its brackets and nothing odd inside.
func validMsgIDText(id string) bool {
	return len(id) > 2 && id[0] == '<' && id[len(id)-1] == '>' && !strings.ContainsAny(id[1:len(id)-1], "<> \t\r\n")
}

// TestDcanlockCancelNewsgroups: without -n the Newsgroups come from dreadart, which sits next to
// dcanlock; a copy of dcanlock next to a stand-in dreadart shows how its output is read.
func TestDcanlockCancelNewsgroups(t *testing.T) {
	s := dcanlockSite(t, vsBoth)
	dir := t.TempDir()
	src, err := os.ReadFile(filepath.Join(s.Bin, "dcanlock"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "dcanlock"), src, 0o755); err != nil {
		t.Fatal(err)
	}
	fake := "#!/bin/sh\n[ \"$1\" = -h ] || exit 3\ncase \"$2\" in *missing*) exit 1;; esac\n" +
		"printf 'Path: x\\r\\nNewsgroups: e2e.test,\\r\\n e2e.other\\r\\nSubject: s\\r\\n\\r\\n'\n"
	if err := os.WriteFile(filepath.Join(dir, "dreadart"), []byte(fake), 0o755); err != nil {
		t.Fatal(err)
	}
	run := func(mid string) (string, int) {
		cmd := exec.Command(filepath.Join(dir, "dcanlock"), "cancel", mid)
		cmd.Dir = s.Dir
		cmd.Env = append(os.Environ(), s.Env...)
		var out bytes.Buffer
		cmd.Stdout, cmd.Stderr = &out, &out
		err := cmd.Run()
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return out.String(), ee.ExitCode()
		}
		return out.String(), 0
	}
	out, code := run("<found@example>")
	head, _ := splitCancel(t, out)
	if code != 0 || headerValue(head, "Newsgroups") != "e2e.test,e2e.other" {
		t.Errorf("exit %d, Newsgroups %q", code, headerValue(head, "Newsgroups"))
	}
	if out, code := run("<missing@example>"); code != 2 || !strings.Contains(out, "give them with -n") {
		t.Errorf("a missing article: exit %d, output %q", code, out)
	}

	// a Newsgroups list that does not fit is an error, not a cut list that names other groups
	var groups []string
	for i := 0; len(strings.Join(groups, ",")) < 1200; i++ {
		groups = append(groups, fmt.Sprintf("e2e.group%03d", i))
	}
	long := "#!/bin/sh\nprintf 'Newsgroups: " + strings.Join(groups, ",") + "\\r\\n\\r\\n'\n"
	if err := os.WriteFile(filepath.Join(dir, "dreadart"), []byte(long), 0o755); err != nil {
		t.Fatal(err)
	}
	if out, code := run("<found@example>"); code != 2 || !strings.Contains(out, "give them with -n") {
		t.Errorf("a Newsgroups list of %d bytes: exit %d, output %q", len(strings.Join(groups, ",")), code, out)
	}
}

// TestDcanlockCancelDreadartPath: started by name (found through PATH), dcanlock uses the dreadart
// of the directory it was found in, not the first dreadart on PATH.
func TestDcanlockCancelDreadartPath(t *testing.T) {
	s := dcanlockSite(t, vsBoth)
	mine, other := t.TempDir(), t.TempDir()
	src, err := os.ReadFile(filepath.Join(s.Bin, "dcanlock"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(mine, "dcanlock"), src, 0o755); err != nil {
		t.Fatal(err)
	}
	fake := func(groups string) []byte {
		return []byte("#!/bin/sh\nprintf 'Newsgroups: " + groups + "\\r\\n\\r\\n'\n")
	}
	if err := os.WriteFile(filepath.Join(mine, "dreadart"), fake("e2e.mine"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(other, "dreadart"), fake("e2e.other"), 0o755); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(filepath.Join(mine, "dcanlock"), "cancel", "<x@example>")
	cmd.Args[0] = "dcanlock" // as the shell passes it when it found the program through PATH
	cmd.Dir = s.Dir
	for _, e := range append(os.Environ(), s.Env...) {
		if !strings.HasPrefix(e, "PATH=") {
			cmd.Env = append(cmd.Env, e)
		}
	}
	// the other dreadart comes first on PATH, but dcanlock is only in mine
	cmd.Env = append(cmd.Env, "PATH="+other+":"+mine+":/usr/bin:/bin")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("dcanlock cancel: %v\n%s", err, out)
	}
	head, _ := splitCancel(t, string(out))
	if got := headerValue(head, "Newsgroups"); got != "e2e.mine" {
		t.Errorf("Newsgroups %q: not the dreadart next to dcanlock (want e2e.mine)", got)
	}
}

// postRaw posts raw (an article as a program wrote it, lines ending in CR LF) the way a client
// that only appends the terminating dot would, and returns the headers the outgoing server got.
func (g *genSite) postRaw(tb testing.TB, c *client.Conn, raw string) []string {
	tb.Helper()
	g.mu.Lock()
	n := len(g.arts)
	g.mu.Unlock()
	expect(tb, c, 340, "POST")
	send(tb, c, raw+".\r\n")
	status(tb, c, 240, "POST article")
	waitFor(tb, 15*time.Second, "the outgoing server to receive the post", func() bool {
		g.mu.Lock()
		defer g.mu.Unlock()
		return len(g.arts) > n
	})
	g.mu.Lock()
	defer g.mu.Unlock()
	head, _ := splitArticle(g.arts[n])
	return head
}

// TestDcanlockCancelPost: the output of dcanlock cancel is an article that dreaderd takes as it
// is (LF-only lines made the whole article one header and got 441), and the cancel that the
// reader then sends out withdraws the article.
func TestDcanlockCancelPost(t *testing.T) {
	skipWithoutDcanlock(t)
	g := newGenSite(t, true, vsBoth, "    controlpost yes\n    approve yes\n", "require-auth")
	mid := msgID(t)
	g.post(t, g.poster(t), "Message-ID: "+mid)
	r := g.Reader(t)
	expect(t, r, 381, "AUTHINFO USER fred")
	expect(t, r, 281, "AUTHINFO PASS fredspass")
	feedAndSettle(t, g.Site, g.st, r, fromElsewhere(g.last()))
	if !present(t, r, 1) {
		t.Fatal("the posted article is not article 1 of e2e.test")
	}
	out, code := dcanlock(t, g.Site, "", "-f", "admin@example.invalid", "-n", "e2e.test", "cancel", mid)
	if code != 0 {
		t.Fatalf("dcanlock cancel: exit %d: %s", code, out)
	}
	head := g.postRaw(t, g.poster(t), out+"\r\n")
	if got := headerValue(head, "Control"); got != "cancel "+mid {
		t.Fatalf("Control %q in the posted cancel: %q", got, head)
	}
	cancel := fromElsewhere(append(slices.Clone(head), "", "Admin cancel."))
	feedAndSettle(t, g.Site, g.st, r, cancel)
	if present(t, r, 1) {
		t.Error("the article is still there after the cancel that dcanlock wrote and dreaderd posted")
	}
}

// TestDcanlockCancelWithdraw: the whole use case with the whole article: an article posted
// through dreaderd is withdrawn by the cancel article dcanlock wrote, fed to a reader in
// require-auth mode.
func TestDcanlockCancelWithdraw(t *testing.T) {
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
	out, code := dcanlock(t, g.Site, "", "-f", "admin@example.invalid", "-n", "e2e.test", "cancel", mid)
	if code != 0 {
		t.Fatalf("dcanlock cancel: exit %d: %s", code, out)
	}
	feedAndSettle(t, g.Site, g.st, r, strings.Split(strings.ReplaceAll(strings.TrimRight(out, "\r\n"), "\r\n", "\n"), "\n"))
	if present(t, r, 1) {
		t.Error("the article is still there after the cancel article of dcanlock")
	}
}

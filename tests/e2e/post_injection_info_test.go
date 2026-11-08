package e2e

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	testkit "github.com/go-while/nntp-testkit"
	"github.com/go-while/nntp-testkit/client"
	"github.com/go-while/nntp-testkit/fakeserver"
)

// injAccess is DefaultReaderAccess with "rfc5537 yes" and these vserverdef lines added.
func injAccess(vs string) string {
	return plainAccess("    rfc5537 yes\n" + vs)
}

// plainAccess is DefaultReaderAccess with these vserverdef lines added.
func plainAccess(vs string) string {
	return strings.Replace(DefaultReaderAccess, "    hostname "+PathHost+"\n",
		"    hostname "+PathHost+"\n"+vs, 1)
}

// postHead posts an article with these header lines through a reader with access and
// returns the header the outgoing server got, with folded lines joined.
func postHead(t *testing.T, access string, hdrs ...string) []string {
	t.Helper()
	return postHeadLogin(t, access, "", "", hdrs...)
}

// postHeadLogin is postHead after AUTHINFO USER/PASS (none when user is "").
func postHeadLogin(t *testing.T, access, user, pass string, hdrs ...string) []string {
	t.Helper()
	_, spool := spoolStore(t)
	var mu sync.Mutex
	var arts [][]string
	post := fakeserver.New(t, fakeserver.Options{Handler: fakeserver.PeerHandler(&fakeserver.PeerOptions{
		OnArticle: func(id string, wire []string) {
			mu.Lock()
			defer mu.Unlock()
			arts = append(arts, wire)
		},
	})})
	s := NewReaderSite(t, ReaderOptions{Spool: spool, Post: post, Access: access})
	c := s.Reader(t)
	if user != "" {
		c.ExpectCmd(381, "AUTHINFO USER %s", user)
		c.ExpectCmd(281, "AUTHINFO PASS %s", pass)
	}
	c.ExpectCmd(340, "POST")
	c.MustSend(client.Stuff(append(append([]string{
		"From: E2E Poster <poster@example.invalid>",
		"Newsgroups: e2e.test",
		"Subject: injection info",
	}, hdrs...), "", "body")))
	c.ExpectStatus(240, "POST")
	testkit.WaitFor(t, 15*time.Second, "the outgoing server to receive the post", func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(arts) > 0
	})
	mu.Lock()
	head, _ := splitArticle(slices.Clone(arts[0]))
	mu.Unlock()
	var joined []string
	for _, l := range head {
		if (strings.HasPrefix(l, " ") || strings.HasPrefix(l, "\t")) && len(joined) > 0 {
			joined[len(joined)-1] += " " + strings.TrimSpace(l)
			continue
		}
		joined = append(joined, l)
	}
	return joined
}

// loginAccess is AuthReaderAccess with rfc5537, a postpath and these vserverdef lines.
func loginAccess(vs string) string {
	return strings.Replace(AuthReaderAccess, "    hostname "+PathHost+"\n",
		"    hostname "+PathHost+"\n    rfc5537 yes\n    postpath "+PathHost+"\n"+vs, 1)
}

// injParam is the value of an Injection-Info parameter, unquoted ("" if there is none).
func injParam(ii, name string) string {
	m := regexp.MustCompile(`;\s*` + regexp.QuoteMeta(name) + `="((?:[^"\\]|\\.)*)"`).FindStringSubmatch(ii)
	if m == nil {
		return ""
	}
	return regexp.MustCompile(`\\(.)`).ReplaceAllString(m[1], "$1")
}

// isHostValue: posting-host's <host-value> for the test client, 127.0.0.1 or name:127.0.0.1.
func isHostValue(v string) bool {
	return regexp.MustCompile(`^([A-Za-z0-9.-]+:)?127\.0\.0\.1$`).MatchString(v)
}

// cryptpwDecrypt reads back a value dreaderd encrypted with a cryptpw: base64 of the
// openssl enc format ("Salted__", 8 bytes salt, AES-256-CBC), key and IV from
// EVP_BytesToKey with SHA-256 and one round, as "openssl enc -d -aes-256-cbc -md sha256".
func cryptpwDecrypt(t *testing.T, pw, b64 string) string {
	t.Helper()
	raw, err := base64.StdEncoding.DecodeString(b64)
	if err != nil || len(raw) < 32 || string(raw[:8]) != "Salted__" || (len(raw)-16)%aes.BlockSize != 0 {
		t.Errorf("%q is not a salted openssl enc value (%v)", b64, err)
		return ""
	}
	out, ok := cryptpwOpen(pw, raw[8:16], raw[16:])
	if !ok {
		t.Errorf("%q does not decrypt with the cryptpw", b64)
		return ""
	}
	return string(out)
}

// cryptpwKey is EVP_BytesToKey with SHA-256 and one round: the AES-256 key and the IV.
func cryptpwKey(pw string, salt []byte) (cipher.Block, []byte) {
	var kiv, d []byte
	for len(kiv) < 48 {
		h := sha256.Sum256(append(append(append([]byte{}, d...), pw...), salt...))
		d = h[:]
		kiv = append(kiv, d...)
	}
	blk, _ := aes.NewCipher(kiv[:32])
	return blk, kiv[32:48]
}

// cryptpwOpen decrypts enc with pw and salt; ok is false when the padding is not valid.
func cryptpwOpen(pw string, salt, enc []byte) ([]byte, bool) {
	blk, iv := cryptpwKey(pw, salt)
	out := make([]byte, len(enc))
	cipher.NewCBCDecrypter(blk, iv).CryptBlocks(out, enc)
	n := int(out[len(out)-1])
	if n < 1 || n > aes.BlockSize || !bytes.Equal(out[len(out)-n:], bytes.Repeat([]byte{byte(n)}, n)) {
		return nil, false
	}
	return out[:len(out)-n], true
}

// cryptpwSeal encrypts plain with pw and salt as dreaderd does: base64 of "Salted__", the salt
// and the AES-256-CBC text with PKCS#7 padding.
func cryptpwSeal(pw string, salt []byte, plain string) string {
	blk, iv := cryptpwKey(pw, salt)
	n := aes.BlockSize - len(plain)%aes.BlockSize
	in := append([]byte(plain), bytes.Repeat([]byte{byte(n)}, n)...)
	out := make([]byte, len(in))
	cipher.NewCBCEncrypter(blk, iv).CryptBlocks(out, in)
	return base64.StdEncoding.EncodeToString(append(append([]byte("Salted__"), salt...), out...))
}

// decryptScript runs contrib/xtrace/decrypt-aes.sh with args on the header and returns what it
// printed; ok is false (and the check skipped) without openssl.
func decryptScript(t *testing.T, head []string, args ...string) (string, bool) {
	t.Helper()
	if _, err := exec.LookPath("openssl"); err != nil {
		t.Log("no openssl: contrib/xtrace/decrypt-aes.sh not run")
		return "", false
	}
	out, err := runDecryptScript(t, head, args...)
	if err != nil {
		t.Errorf("contrib/xtrace/decrypt-aes.sh: %v: %s", err, out)
	}
	return out, true
}

// runDecryptScript runs contrib/xtrace/decrypt-aes.sh with args on the header.
func runDecryptScript(t *testing.T, head []string, args ...string) (string, error) {
	t.Helper()
	cmd := exec.Command("sh", append([]string{filepath.Join(repoRoot(t), "contrib", "xtrace", "decrypt-aes.sh")}, args...)...)
	cmd.Stdin = strings.NewReader(strings.Join(head, "\r\n") + "\r\n\r\nbody\r\n")
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// TestDecryptAesWrongCryptpw (D-201): contrib/xtrace/decrypt-aes.sh must not take a wrong
// cryptpw's output as the value. A wrong password gives valid CBC padding about 1 time in
// 256; the script took the first cryptpw of dreader.access whose output had no control
// characters, and it let every byte above 127 through. So with another vserverdef's cryptpw
// listed first, a short value read back as garbage about once in 2,000 posts (the
// TestReaderPostInjectionInfo/cryptpw flake). The test finds a salt for which "fred",
// encrypted with "secret", decrypts with "another" to such garbage (not valid UTF-8), and
// checks that the script prints fred with both cryptpws listed, in either order, and refuses
// the value with the wrong password alone. Fails before D-201: the garbage was printed.
func TestDecryptAesWrongCryptpw(t *testing.T) {
	if _, err := exec.LookPath("openssl"); err != nil {
		t.Skip("no openssl: contrib/xtrace/decrypt-aes.sh not run")
	}
	var value string
	var garbage []byte
	salt := make([]byte, 8)
	for i := uint64(1); i < 1<<22 && value == ""; i++ {
		binary.BigEndian.PutUint64(salt, i)
		enc := cryptpwSeal("secret", salt, "fred")
		raw, _ := base64.StdEncoding.DecodeString(enc)
		p, ok := cryptpwOpen("another", salt, raw[16:])
		if !ok || len(p) == 0 || utf8.Valid(p) {
			continue
		}
		high, ctl := false, false
		for _, b := range p {
			high = high || b >= 0x80
			ctl = ctl || b < 0x20 || b == 0x7f
		}
		if high && !ctl {
			value, garbage = enc, p
		}
	}
	if value == "" {
		t.Fatal("no salt found for which the wrong cryptpw gives printable garbage")
	}
	t.Logf("salt %x: \"another\" decrypts %s to %q", salt, value, garbage)
	head := []string{`Injection-Info: e2e.test; posting-account="C=` + value + `"`}
	dir := t.TempDir()
	for _, order := range [][2]string{{"another", "secret"}, {"secret", "another"}} {
		acc := filepath.Join(dir, "dreader.access."+order[0])
		text := "vserverdef other\n    cryptpw " + order[0] + "\nend\nvserverdef DEFAULT\n    cryptpw " + order[1] + "\nend\n"
		if err := os.WriteFile(acc, []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
		out, err := runDecryptScript(t, head, "-a", acc)
		if err != nil || !strings.Contains(out, "Injection-Info posting-account: fred\n") || strings.Contains(out, string(garbage)) {
			t.Errorf("cryptpw %s listed first: %v, printed %q, want posting-account fred", order[0], err, out)
		}
	}
	out, err := runDecryptScript(t, head, "pass:another")
	if err == nil || !strings.Contains(out, "cannot decrypt") || strings.Contains(out, string(garbage)) {
		t.Errorf("pass:another: %v, printed %q, want \"cannot decrypt\" and exit status 1", err, out)
	}
}

func hasHeader(head []string, name string) bool {
	for _, l := range head {
		if k, _, ok := strings.Cut(l, ":"); ok && strings.EqualFold(k, name) {
			return true
		}
	}
	return false
}

var injDateRe = regexp.MustCompile(`^(Mon|Tue|Wed|Thu|Fri|Sat|Sun), [0-3][0-9] (Jan|Feb|Mar|Apr|May|Jun|Jul|Aug|Sep|Oct|Nov|Dec) [0-9]{4} [0-2][0-9]:[0-5][0-9]:[0-6][0-9] \+0000 \(UTC\)$`)

// TestReaderPostInjectionInfo (F-07 step 2, RFC 5537 3.5 steps 9 to 11): with "rfc5537 yes" a
// post gets a Path with the "!.POSTED" diagnostic, an Injection-Info with posting-host and
// mail-complaints-to (no logging-data), and no NNTP-Posting-Host, X-Trace or X-Complaints-To;
// "showposthost no" leaves posting-host out, and with a cryptpw posting-host, posting-account
// and X-Trace decrypt with it (also with contrib/xtrace/decrypt-aes.sh).
// Injection-Date is added only where dreaderd supplied the Message-ID or Date, and a poster's
// own is kept. With "rfc5537 no" and with "legacytrace yes", the old trace headers
// are written and no Injection-* header.
func TestReaderPostInjectionInfo(t *testing.T) {
	access := injAccess("    postpath " + PathHost + "\n    abuseto abuse@example.invalid\n")

	for _, mode := range []string{"rfc5537 yes", "default", "legacytrace no"} {
		t.Run(mode, func(t *testing.T) {
			access := strings.Replace(access, "    rfc5537 yes\n", "", 1)
			if mode != "default" {
				access = strings.Replace(access, "    hostname "+PathHost+"\n", "    hostname "+PathHost+"\n    "+mode+"\n", 1)
			}
			head := postHead(t, access, "Message-ID: "+msgID(t), "Date: "+time.Now().UTC().Format(time.RFC1123Z))
			if p := headerValue(head, "Path"); p != PathHost+"!.POSTED!not-for-mail" {
				t.Errorf("Path %q, want %s!.POSTED!not-for-mail", p, PathHost)
			}
			ii := headerValue(head, "Injection-Info")
			for _, want := range []string{PathHost + ";", `posting-host="`, "127.0.0.1", `mail-complaints-to="abuse@example.invalid"`} {
				if !strings.Contains(ii, want) {
					t.Errorf("Injection-Info %q, want it to contain %s", ii, want)
				}
			}
			if !strings.HasPrefix(ii, PathHost+";") {
				t.Errorf("Injection-Info %q does not start with the path-identity %s", ii, PathHost)
			}
			// posting-host is a <host-value>: the address, or name:address, without a port
			if m := regexp.MustCompile(`posting-host="([^"]*)"`).FindStringSubmatch(ii); m == nil ||
				!(m[1] == "127.0.0.1" || regexp.MustCompile(`^[A-Za-z0-9.-]+:127\.0\.0\.1$`).MatchString(m[1])) {
				t.Errorf("Injection-Info %q: posting-host is not 127.0.0.1 or name:127.0.0.1", ii)
			}
			// the "post ok" log line has the Message-ID, which finds the post: no logging-data
			if strings.Contains(ii, "logging-data=") {
				t.Errorf("Injection-Info %q has a logging-data", ii)
			}
			if strings.Contains(ii, "posting-account=") {
				t.Errorf("Injection-Info %q has a posting-account without showposter yes", ii)
			}
			for _, h := range []string{"NNTP-Posting-Host", "X-Trace", "X-Complaints-To", "Injection-Date"} {
				if hasHeader(head, h) {
					t.Errorf("post has %s: %q (Message-ID and Date were the poster's)", h, headerValue(head, h))
				}
			}
		})
	}

	t.Run("dreaderd adds the Message-ID", func(t *testing.T) {
		head := postHead(t, access)
		if d := headerValue(head, "Injection-Date"); !injDateRe.MatchString(d) {
			t.Errorf("Injection-Date %q, want an RFC 5322 date in +0000", d)
		}
	})

	t.Run("the poster's Injection-Date is kept", func(t *testing.T) {
		const own = "Mon, 02 Nov 2026 10:00:00 +0000"
		head := postHead(t, access, "Injection-Date: "+own)
		var dates []string
		for _, l := range head {
			if k, v, ok := strings.Cut(l, ":"); ok && strings.EqualFold(k, "Injection-Date") {
				dates = append(dates, strings.TrimSpace(v))
			}
		}
		if len(dates) != 1 || dates[0] != own {
			t.Errorf("Injection-Date %q, want only the poster's %q", dates, own)
		}
	})

	t.Run("cryptpw", func(t *testing.T) {
		// posting-host and posting-account are encrypted as X-Trace is, so the admin can
		// read them back with the cryptpw (the P= part of "G=gen,P=password")
		for _, c := range []struct{ pw, gen string }{{"secret", ""}, {"G=7,P=secret", "G=7,"}} {
			head := postHeadLogin(t, loginAccess("    showposter yes\n    cryptpw "+c.pw+"\n"), "fred", "fredspass", "Message-ID: "+msgID(t))
			ii := headerValue(head, "Injection-Info")
			if strings.Contains(ii, "127.0.0.1") || strings.Contains(ii, "fred") || strings.Contains(ii, "logging-data=") {
				t.Errorf("cryptpw %s: Injection-Info %q shows the address or the login, or has a logging-data", c.pw, ii)
			}
			host := injParam(ii, "posting-host")
			if !strings.HasPrefix(host, c.gen+"C=") {
				t.Errorf("cryptpw %s: posting-host %q, want %sC=<base64>", c.pw, host, c.gen)
			} else if d := cryptpwDecrypt(t, "secret", strings.TrimPrefix(host, c.gen+"C=")); !isHostValue(d) {
				t.Errorf("cryptpw %s: posting-host decrypts to %q, want 127.0.0.1 or name:127.0.0.1", c.pw, d)
			}
			acct := injParam(ii, "posting-account")
			if !strings.HasPrefix(acct, c.gen+"C=") {
				t.Errorf("cryptpw %s: posting-account %q, want %sC=<base64>", c.pw, acct, c.gen)
			} else if d := cryptpwDecrypt(t, "secret", strings.TrimPrefix(acct, c.gen+"C=")); d != "fred" {
				t.Errorf("cryptpw %s: posting-account decrypts to %q, want fred", c.pw, d)
			}
			// the admin's tool reads the same back, with the password or with the cryptpw
			// lines of a dreader.access (another vserverdef's first)
			acc := filepath.Join(t.TempDir(), "dreader.access")
			if err := os.WriteFile(acc, []byte("vserverdef other\n    cryptpw another\nend\nvserverdef DEFAULT\n    cryptpw "+c.pw+"\nend\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			for _, args := range [][]string{{"pass:secret"}, {"-a", acc}} {
				if out, ok := decryptScript(t, head, args...); ok {
					if !strings.Contains(out, "Injection-Info posting-account: fred") ||
						!regexp.MustCompile(`Injection-Info posting-host: ([A-Za-z0-9.-]+:)?127\.0\.0\.1`).MatchString(out) {
						t.Errorf("cryptpw %s: contrib/xtrace/decrypt-aes.sh %v printed %q, want the posting-host and fred", c.pw, args, out)
					}
				}
			}
		}
	})

	t.Run("showposthost", func(t *testing.T) {
		ii := headerValue(postHead(t, injAccess("    postpath "+PathHost+"\n    showposthost no\n"), "Message-ID: "+msgID(t)), "Injection-Info")
		if strings.Contains(ii, "posting-host=") || strings.Contains(ii, "127.0.0.1") || !strings.HasPrefix(ii, PathHost) {
			t.Errorf("showposthost no: Injection-Info %q, want no posting-host", ii)
		}
		ii = headerValue(postHead(t, injAccess("    postpath "+PathHost+"\n    showposthost yes\n"), "Message-ID: "+msgID(t)), "Injection-Info")
		if !isHostValue(injParam(ii, "posting-host")) {
			t.Errorf("showposthost yes: Injection-Info %q, want posting-host 127.0.0.1 or name:127.0.0.1", ii)
		}
	})

	t.Run("cryptpw with rfc5537 no", func(t *testing.T) {
		head := postHead(t, plainAccess("    rfc5537 no\n    cryptpw secret\n"), "Message-ID: "+msgID(t))
		xt := headerValue(head, "X-Trace")
		if !strings.HasPrefix(xt, "C=") {
			t.Fatalf("X-Trace %q, want C=<base64>", xt)
		}
		if d := cryptpwDecrypt(t, "secret", strings.TrimPrefix(xt, "C=")); !strings.Contains(d, " "+PathHost+" ") || !strings.Contains(d, "127.0.0.1") {
			t.Errorf("X-Trace decrypts to %q, want the hostname %s and the address", d, PathHost)
		}
		if out, ok := decryptScript(t, head, "pass:secret"); ok && !regexp.MustCompile(`X-Trace: [0-9]+ `+regexp.QuoteMeta(PathHost)+` .*127\.0\.0\.1`).MatchString(out) {
			t.Errorf("contrib/xtrace/decrypt-aes.sh printed %q, want the X-Trace data", out)
		}
	})

	t.Run("showposter", func(t *testing.T) {
		login := loginAccess
		ii := headerValue(postHeadLogin(t, login("    showposter yes\n"), "fred", "fredspass"), "Injection-Info")
		if !strings.Contains(ii, `posting-account="fred"`) {
			t.Errorf("showposter yes, logged in as fred: Injection-Info %q, want posting-account=\"fred\"", ii)
		}
		ii = headerValue(postHeadLogin(t, login(""), "fred", "fredspass"), "Injection-Info")
		if strings.Contains(ii, "posting-account=") {
			t.Errorf("without showposter: Injection-Info %q has a posting-account", ii)
		}
		ii = headerValue(postHeadLogin(t, login("    showposter yes\n    cryptpw secret\n"), "fred", "fredspass"), "Injection-Info")
		if !strings.Contains(ii, `posting-account="C=`) || strings.Contains(ii, "fred") {
			t.Errorf("showposter yes with a cryptpw: Injection-Info %q, want an encrypted posting-account", ii)
		}
	})

	t.Run("no postpath", func(t *testing.T) {
		head := postHead(t, injAccess(""), "Message-ID: "+msgID(t))
		if p := headerValue(head, "Path"); p != "not-for-mail" {
			t.Errorf("Path %q without a postpath, want not-for-mail", p)
		}
		if ii := headerValue(head, "Injection-Info"); !strings.HasPrefix(ii, PathHost+";") {
			t.Errorf("Injection-Info %q, want the hostname %s as path-identity", ii, PathHost)
		}
	})

	t.Run("postpath with two elements", func(t *testing.T) {
		// the Path grows to the left: the injecting agent is the element right before
		// ".POSTED", and Injection-Info names that one
		head := postHead(t, injAccess("    postpath outer.example!"+PathHost+"\n"), "Message-ID: "+msgID(t))
		if p := headerValue(head, "Path"); p != "outer.example!"+PathHost+"!.POSTED!not-for-mail" {
			t.Errorf("Path %q, want outer.example!%s!.POSTED!not-for-mail", p, PathHost)
		}
		if ii := headerValue(head, "Injection-Info"); !strings.HasPrefix(ii, PathHost+";") {
			t.Errorf("Injection-Info %q, want the element before .POSTED (%s) as path-identity", ii, PathHost)
		}
	})

	for _, c := range []struct{ name, vs string }{
		{"rfc5537 no", "    rfc5537 no\n"},
		{"legacytrace yes", "    rfc5537 yes\n    legacytrace yes\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			head := postHead(t, plainAccess("    postpath "+PathHost+"\n    abuseto abuse@example.invalid\n"+c.vs))
			if p := headerValue(head, "Path"); p != PathHost+"!not-for-mail" {
				t.Errorf("Path %q, want %s!not-for-mail", p, PathHost)
			}
			for _, h := range []string{"NNTP-Posting-Host", "X-Trace", "X-Complaints-To"} {
				if !hasHeader(head, h) {
					t.Errorf("no %s header", h)
				}
			}
			for _, h := range []string{"Injection-Info", "Injection-Date"} {
				if hasHeader(head, h) {
					t.Errorf("post has %s: %q", h, headerValue(head, h))
				}
			}
		})
	}
}

// TestReaderModeratorGetsNoInjectionInfo (F-07, RFC 5537 3.5 step 7): a post to a moderated
// group is forwarded before Injection-Info and Injection-Date are added, so the moderator's
// copy has neither and the moderator's own post of it is not refused as already injected.
func TestReaderModeratorGetsNoInjectionInfo(t *testing.T) {
	m := newModerationReader(t, ModOptions{Access: injAccess("    postpath " + PathHost + "\n")})
	if m.Mail == nil {
		t.Skip("needs ci/fake-sendmail as the system's sendmail")
	}
	c := m.Reader(t)
	c.ExpectCmd(340, "POST")
	c.MustSend(client.Stuff([]string{
		"From: E2E Poster <poster@example.invalid>",
		"Newsgroups: " + ModGroup,
		"Subject: for the moderator",
		"",
		"body",
	}))
	c.ExpectStatus(240, "POST to a moderated group")
	mails := m.Mail.waitMails(t, 1)
	head, _, _ := strings.Cut(mails[0].Msg, "\n\n")
	for _, h := range []string{"Injection-Info:", "Injection-Date:"} {
		if strings.Contains(strings.ToLower(head), strings.ToLower(h)) {
			t.Errorf("the moderator's copy has %s\n%s", h, head)
		}
	}
	if !strings.Contains(head, "Message-ID:") {
		t.Errorf("the moderator's copy has no Message-ID\n%s", head)
	}
}

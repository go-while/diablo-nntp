package e2e

// Cancel-Lock generation (RFC 8315): dreaderd adds Cancel-Lock to articles posted through it
// and Cancel-Key to a poster's own cancels and supersedes, from the canlockadmin and
// canlockuser secrets of the vserverdef. Go computes the expected values with canKey and
// canLock (pinned to INN's vectors in canlock_test.go).

import (
	"bytes"
	"crypto/sha1"
	"crypto/sha256"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-while/nntp-testkit/client"
	"github.com/go-while/nntp-testkit/fakeserver"
)

const (
	genAdminSecret = "e2e-admin-secret"
	genUserSecret  = "e2e-user-secret"
	genOldSecret   = "e2e-user-secret-rotated-out"
)

// skipWithoutCanLockGenerate skips the test unless the dreaderd in DIABLO_BIN knows canlockadmin:
// without Cancel-Lock generation it logs the line as an invalid vserverdef option and adds nothing.
func skipWithoutCanLockGenerate(tb testing.TB) {
	tb.Helper()
	b, err := os.ReadFile(filepath.Join(bin(tb), "dreaderd"))
	if err != nil || !bytes.Contains(b, []byte("canlockadmin")) {
		tb.Skip("dreaderd does not generate Cancel-Lock (canlockadmin)")
	}
}

// genAccess is a dreader.access that lets 127.0.0.1 post, as fred (auth) or anonymously, and
// PeerIP feed. vs and rd are extra lines for the vserverdef and the poster's readerdef.
func genAccess(auth bool, vs, rd string) string {
	poster := "readerdef poster\n    read yes\n    post yes\n"
	if auth {
		poster += "    auth fred\n"
	}
	return "vserverdef DEFAULT\n    hostname " + PathHost + "\n    postpath " + PathHost + "\n" + vs + "end\n" +
		"authdef fred\n    user fred\n    pass fredspass\nend\n" +
		poster + rd + "end\n" +
		"readerdef feed\n    feed yes\nend\n" +
		"access 127.0.0.1 poster\naccess " + PeerIP + " feed\n"
}

// genSite is a reader site whose posts go to a fake server that records them.
type genSite struct {
	*Site
	st   *fakeserver.Store
	auth bool
	mu   sync.Mutex
	arts [][]string
}

// newGenSite starts a reader with the given vserverdef and readerdef lines. mode is the
// readercanlock mode ("" = the default), with a dcontrol.ctl that executes cancels.
func newGenSite(tb testing.TB, auth bool, vs, rd, mode string) *genSite {
	tb.Helper()
	skipWithoutCanLockGenerate(tb)
	g := &genSite{auth: auth}
	st, spool := spoolStore(tb)
	g.st = st
	post := fakeserver.New(tb, fakeserver.Options{Handler: fakeserver.PeerHandler(&fakeserver.PeerOptions{
		OnArticle: func(id string, wire []string) {
			g.mu.Lock()
			defer g.mu.Unlock()
			g.arts = append(g.arts, wire)
		},
	})})
	o := ReaderOptions{Spool: spool, Post: post, Access: genAccess(auth, vs, rd), NoStart: true,
		Files: map[string]string{"dcontrol.ctl": "cancel:*:*:doit,break\n"}}
	if mode != "" {
		o.Config = map[string]string{"readercanlock": mode}
	}
	g.Site = NewReaderSite(tb, o)
	_, port, _ := strings.Cut(g.Addr, ":")
	startReader(tb, g.Site, "[127.0.0.1]:"+port)
	return g
}

// poster returns a connection that is logged in as fred when the site authenticates.
func (g *genSite) poster(tb testing.TB) *client.Conn {
	tb.Helper()
	c := g.Reader(tb)
	if g.auth {
		expect(tb, c, 381, "AUTHINFO USER fred")
		expect(tb, c, 281, "AUTHINFO PASS fredspass")
	}
	return c
}

// post posts hdrs and a one line body and returns the headers of the article as the outgoing
// server received it.
func (g *genSite) post(tb testing.TB, c *client.Conn, hdrs ...string) []string {
	tb.Helper()
	g.mu.Lock()
	n := len(g.arts)
	g.mu.Unlock()
	expect(tb, c, 340, "POST")
	send(tb, c, wire(append(append([]string{
		"From: E2E Poster <poster@example.invalid>",
		"Newsgroups: e2e.test",
		"Subject: lock test",
	}, hdrs...), "", "body")))
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

// fromElsewhere replaces the Path of a posted article so that it can be fed back as if it came
// from another server (a Path naming this server is a loop).
func fromElsewhere(art []string) []string {
	out := slices.Clone(art)
	for i, l := range out {
		if strings.HasPrefix(l, "Path:") {
			out[i] = "Path: e2e-remote!not-for-mail"
		}
	}
	return out
}

// last returns the last article the outgoing server received.
func (g *genSite) last() []string {
	g.mu.Lock()
	defer g.mu.Unlock()
	return slices.Clone(g.arts[len(g.arts)-1])
}

// headerCount counts the header fields called name.
func headerCount(head []string, name string) int {
	n := 0
	for _, h := range head {
		if k, _, ok := strings.Cut(h, ":"); ok && strings.EqualFold(k, name) {
			n++
		}
	}
	return n
}

// elements splits a Cancel-Lock or Cancel-Key value into its sorted elements.
func elements(v string) []string {
	e := strings.Fields(v)
	slices.Sort(e)
	return e
}

// wantLocks returns the sorted lock elements for secret and uid. sha1 adds the sha1 ones.
func wantLocks(secret, uid, mid string, sha1On bool) []string {
	e := []string{"sha256:" + canLock(sha256.New, canKey(sha256.New, secret, uid, mid))}
	if sha1On {
		e = append(e, "sha1:"+canLock(sha1.New, canKey(sha1.New, secret, uid, mid)))
	}
	return e
}

// wantKeys returns the key elements for secret and uid.
func wantKeys(secret, uid, mid string, sha1On bool) []string {
	e := []string{"sha256:" + canKey(sha256.New, secret, uid, mid)}
	if sha1On {
		e = append(e, "sha1:"+canKey(sha1.New, secret, uid, mid))
	}
	return e
}

func sorted(parts ...[]string) []string {
	var all []string
	for _, p := range parts {
		all = append(all, p...)
	}
	slices.Sort(all)
	return all
}

const (
	vsBoth = "    canlockadmin " + genAdminSecret + "\n    canlockuser " + genUserSecret + "\n"
	vsNoSh = vsBoth + "    canlocksha1 no\n"
)

// TestCanLockGenerate: the Cancel-Lock header added to a post, per configuration.
func TestCanLockGenerate(t *testing.T) {
	type tc struct {
		name   string
		auth   bool
		vs, rd string
		hdrs   []string // extra headers of the post
		want   func(mid string) []string
	}
	both := func(mid string) []string {
		return sorted(wantLocks(genAdminSecret, "", mid, true), wantLocks(genUserSecret, "fred", mid, true))
	}
	for _, c := range []tc{
		{"admin and user, sha256 and sha1", true, vsBoth, "", nil, both},
		{"sha1 off", true, vsNoSh, "", nil, func(mid string) []string {
			return sorted(wantLocks(genAdminSecret, "", mid, false), wantLocks(genUserSecret, "fred", mid, false))
		}},
		{"anonymous poster: admin lock only", false, vsBoth, "", nil, func(mid string) []string {
			return sorted(wantLocks(genAdminSecret, "", mid, true))
		}},
		{"addcanlockuser none", true, vsBoth, "    addcanlockuser none\n", nil, func(mid string) []string {
			return sorted(wantLocks(genAdminSecret, "", mid, true))
		}},
		{"addcanlockuser ip", false, vsBoth, "    addcanlockuser ip\n", nil, func(mid string) []string {
			return sorted(wantLocks(genAdminSecret, "", mid, true), wantLocks(genUserSecret, "127.0.0.1", mid, true))
		}},
		{"user secret only", true, "    canlockuser " + genUserSecret + "\n", "", nil, func(mid string) []string {
			return sorted(wantLocks(genUserSecret, "fred", mid, true))
		}},
		{"admin secret only", true, "    canlockadmin " + genAdminSecret + "\n", "", nil, func(mid string) []string {
			return sorted(wantLocks(genAdminSecret, "", mid, true))
		}},
		{"rotated secrets: locks from the first only", true, vsBoth + "    canlockuser " + genOldSecret + "\n", "", nil, both},
	} {
		t.Run(c.name, func(t *testing.T) {
			g := newGenSite(t, c.auth, c.vs, c.rd, "")
			mid := msgID(t)
			head := g.post(t, g.poster(t), append([]string{"Message-ID: " + mid}, c.hdrs...)...)
			if n := headerCount(head, "Cancel-Lock"); n != 1 {
				t.Fatalf("%d Cancel-Lock headers in %q, want 1", n, head)
			}
			if want := c.want(mid); !slices.Equal(elements(headerValue(head, "Cancel-Lock")), want) {
				got := elements(headerValue(head, "Cancel-Lock"))
				t.Errorf("Cancel-Lock elements %q, want %q", got, want)
			}
		})
	}
}

// TestCanLockGenerateOff: without secrets nothing is added, and the poster's own Cancel-Lock
// and Cancel-Key pass unchanged.
func TestCanLockGenerateOff(t *testing.T) {
	skipWithoutCanLockGenerate(t)
	g := newGenSite(t, true, "", "", "")
	mid := msgID(t)
	mine := "sha256:" + canLock(sha256.New, canKey(sha256.New, "client-secret", "", mid))
	head := g.post(t, g.poster(t), "Message-ID: "+mid, "Cancel-Lock: "+mine)
	if got := headerValue(head, "Cancel-Lock"); got != mine || headerCount(head, "Cancel-Lock") != 1 {
		t.Errorf("Cancel-Lock %q (%d headers), want %q once", got, headerCount(head, "Cancel-Lock"), mine)
	}
	head = g.post(t, g.poster(t), "Message-ID: "+msgID(t))
	if headerCount(head, "Cancel-Lock") != 0 {
		t.Errorf("a Cancel-Lock was added without secrets: %q", head)
	}
	head = g.post(t, g.poster(t), "Message-ID: "+msgID(t), "Control: cancel "+mid)
	if headerCount(head, "Cancel-Key") != 0 {
		t.Errorf("a Cancel-Key was added without secrets: %q", head)
	}
}

// TestCanLockGenerateOwnLock: a poster's own Cancel-Lock stays, one header only (RFC 8315
// section 2), with the admin locks added after it and no user lock.
func TestCanLockGenerateOwnLock(t *testing.T) {
	g := newGenSite(t, true, vsBoth, "", "")
	mid := msgID(t)
	mine := "sha256:" + canLock(sha256.New, canKey(sha256.New, "client-secret", "", mid))
	head := g.post(t, g.poster(t), "Message-ID: "+mid, "Cancel-Lock: "+mine)
	if n := headerCount(head, "Cancel-Lock"); n != 1 {
		t.Fatalf("%d Cancel-Lock headers in %q, want 1", n, head)
	}
	want := sorted([]string{mine}, wantLocks(genAdminSecret, "", mid, true))
	if got := elements(headerValue(head, "Cancel-Lock")); !slices.Equal(got, want) {
		t.Errorf("Cancel-Lock elements %q, want %q", got, want)
	}
	if first := strings.Fields(headerValue(head, "Cancel-Lock"))[0]; first != mine {
		t.Errorf("the poster's own lock is not first: %q", first)
	}
}

// TestCanLockGenerateServerID: the lock is made from the Message-ID the server gives a post
// that has none.
func TestCanLockGenerateServerID(t *testing.T) {
	g := newGenSite(t, true, vsBoth, "", "")
	head := g.post(t, g.poster(t))
	mid := headerValue(head, "Message-ID")
	if mid == "" {
		t.Fatalf("no Message-ID in %q", head)
	}
	want := sorted(wantLocks(genAdminSecret, "", mid, true), wantLocks(genUserSecret, "fred", mid, true))
	if got := elements(headerValue(head, "Cancel-Lock")); !slices.Equal(got, want) {
		t.Errorf("Cancel-Lock elements %q, want %q for %s", got, want, mid)
	}
}

// TestCanLockGenerateKeys: the Cancel-Key added to a poster's cancel or supersede. A rotated
// secret still gets its keys, so articles locked with it can be withdrawn.
func TestCanLockGenerateKeys(t *testing.T) {
	target := msgID(t)
	vs := vsBoth + "    canlockuser " + genOldSecret + "\n"
	for _, c := range []struct {
		name string
		hdr  string
	}{{"cancel", "Control: cancel " + target}, {"supersede", "Supersedes: " + target}} {
		t.Run(c.name, func(t *testing.T) {
			g := newGenSite(t, true, vs, "", "")
			head := g.post(t, g.poster(t), "Message-ID: "+msgID(t), c.hdr)
			want := sorted(wantKeys(genUserSecret, "fred", target, true), wantKeys(genOldSecret, "fred", target, true))
			if n := headerCount(head, "Cancel-Key"); n != 1 {
				t.Fatalf("%d Cancel-Key headers in %q, want 1", n, head)
			}
			if got := elements(headerValue(head, "Cancel-Key")); !slices.Equal(got, want) {
				t.Errorf("Cancel-Key elements %q, want %q", got, want)
			}
		})
	}
	t.Run("poster's own key stays", func(t *testing.T) {
		g := newGenSite(t, true, vsBoth, "", "")
		mine := "sha256:" + canKey(sha256.New, "client-secret", "", target)
		head := g.post(t, g.poster(t), "Message-ID: "+msgID(t), "Control: cancel "+target, "Cancel-Key: "+mine)
		if got := headerValue(head, "Cancel-Key"); got != mine || headerCount(head, "Cancel-Key") != 1 {
			t.Errorf("Cancel-Key %q (%d headers), want %q once", got, headerCount(head, "Cancel-Key"), mine)
		}
	})
	t.Run("anonymous poster gets no key", func(t *testing.T) {
		g := newGenSite(t, false, vsBoth, "", "")
		head := g.post(t, g.poster(t), "Message-ID: "+msgID(t), "Control: cancel "+target)
		if headerCount(head, "Cancel-Key") != 0 {
			t.Errorf("an anonymous cancel got a Cancel-Key: %q", head)
		}
	})
	t.Run("an article that is no cancel gets no key", func(t *testing.T) {
		g := newGenSite(t, true, vsBoth, "", "")
		head := g.post(t, g.poster(t), "Message-ID: "+msgID(t))
		if headerCount(head, "Cancel-Key") != 0 {
			t.Errorf("a plain post got a Cancel-Key: %q", head)
		}
	})
}

// TestCanLockGenerateRoundTrip: a post gets its locks, the poster's cancel gets its keys, and
// a reader in require-auth mode withdraws the article with them: generation and verification
// agree.
func TestCanLockGenerateRoundTrip(t *testing.T) {
	g := newGenSite(t, true, vsBoth, "", "require-auth")
	c := g.poster(t)
	mid := msgID(t)
	g.post(t, c, "Message-ID: "+mid)
	article := fromElsewhere(g.last())
	r := g.Reader(t) // a second connection reads; the poster stays logged in
	if g.auth {
		expect(t, r, 381, "AUTHINFO USER fred")
		expect(t, r, 281, "AUTHINFO PASS fredspass")
	}
	feedAndSettle(t, g.Site, g.st, r, article)
	if !present(t, r, 1) {
		t.Fatal("the posted article is not article 1 of e2e.test")
	}
	g.post(t, c, "Message-ID: "+msgID(t), "Control: cancel "+mid)
	feedAndSettle(t, g.Site, g.st, r, fromElsewhere(g.last()))
	if present(t, r, 1) {
		t.Error("the article is still there after fred's cancel")
	}
}

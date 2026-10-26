package e2e

// dreaderd: the reader. A reader-only site (README.READER topology C) whose spool is a
// fake NNTP server holding the article bodies, and whose outgoing (post) server is a fake
// transit peer.

import (
	"net"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	testkit "github.com/go-while/nntp-testkit"
	"github.com/go-while/nntp-testkit/client"
	"github.com/go-while/nntp-testkit/fakeserver"
	"github.com/go-while/nntp-testkit/proc"
)

// ReaderOptions configures a throwaway dreaderd site.
type ReaderOptions struct {
	Groups []string           // groups created with dgrpctl; nil = e2e.test, e2e.other
	Spool  *fakeserver.Server // the 's' server of dserver.hosts (article bodies)
	// SpoolAddr is the 's' server as host:port when it is not a fakeserver (e.g. a diablo
	// site); one of Spool and SpoolAddr is required.
	SpoolAddr string
	Post      *fakeserver.Server // the 'o' server (POST is sent there with IHAVE); nil = none
	Access    string             // dreader.access; "" = DefaultReaderAccess
	Config    map[string]string  // diablo.config directives added or replacing the generated ones

	// PlainBind passes -P as "127.0.0.1:port" instead of "[127.0.0.1]:port".
	PlainBind bool

	// NoStart writes the site and its groups but doesn't start dreaderd.
	NoStart bool
}

// DefaultReaderAccess lets 127.0.0.1 read and post and PeerIP feed.
const DefaultReaderAccess = `vserverdef DEFAULT
    hostname ` + PathHost + `
end
readerdef rp
    read yes
    post yes
end
readerdef feed
    feed yes
end
access 127.0.0.1 rp
access ` + PeerIP + ` feed
`

// NewReaderSite writes a reader site into a temporary directory, creates its groups,
// starts dreaderd -d (in the foreground) on a free port of 127.0.0.1 and waits for its
// greeting. proc's cleanup stops it with SIGTERM, then SIGKILL.
func NewReaderSite(tb testing.TB, o ReaderOptions) *Site {
	tb.Helper()
	spoolAddr := o.SpoolAddr
	if o.Spool != nil {
		spoolAddr = o.Spool.Addr()
	}
	spoolHost, spoolPort, err := net.SplitHostPort(spoolAddr)
	if err != nil {
		tb.Fatalf("NewReaderSite: Spool or SpoolAddr is required: %v", err)
	}
	s := &Site{Dir: tb.TempDir(), Bin: bin(tb)}
	port := freePort(tb)
	s.Addr = net.JoinHostPort("127.0.0.1", strconv.Itoa(port))
	s.Env = []string{"DIABLO_CONFIG_PATH=" + filepath.Join(s.Dir, "diablo.config")}

	for _, d := range []string{"spool/group", "spool/cache", "spool/postq", "log", "run", "dqueue"} {
		if err := os.MkdirAll(filepath.Join(s.Dir, d), 0o755); err != nil {
			tb.Fatal(err)
		}
	}
	conf := map[string]string{
		"path_home":       s.Dir,
		"readerforks":     "1",
		"readerthreads":   "4",
		"readerdns":       "1",
		"readerfeedforks": "1",
		"readercache":     "off",
	}
	for k, v := range o.Config {
		conf[k] = s.expand(v)
	}
	var b strings.Builder
	b.WriteString("version 2.0\n")
	for _, k := range sortedKeys(conf) {
		b.WriteString(k + "\t" + conf[k] + "\n")
	}
	s.WriteFile(tb, "diablo.config", b.String())
	s.WriteFile(tb, "dreader.access", or(o.Access, DefaultReaderAccess))
	hosts := spoolHost + "\ts1p" + spoolPort + "\n"
	if o.Post != nil {
		// the same host twice: dserver.hosts tells entries apart by the name's case
		hosts += "127.0.0.1\to1p" + strconv.Itoa(o.Post.Port()) + "\n"
	}
	s.WriteFile(tb, "dserver.hosts", hosts)
	s.WriteFile(tb, "dexpire.ctl", "*:x30\n")
	s.WriteFile(tb, "moderators", "")

	// An empty dactive.kp gets its KP header on the first open; dgrpctl does not create it.
	s.WriteFile(tb, "dactive.kp", "")
	groups := o.Groups
	if groups == nil {
		groups = []string{"e2e.test", "e2e.other"}
	}
	for _, g := range groups {
		s.Run(tb, "dgrpctl", "newgroup", g, "y")
	}

	if o.NoStart {
		return s
	}

	// "[host]:port" by default: before the -P fix (jrehmer/diablo-nntp PR "dreaderd: -P/-B
	// host:port") dreaderd kept the ":port" in the host of a plain "host:port".
	bind := "[127.0.0.1]:" + strconv.Itoa(port)
	if o.PlainBind {
		bind = s.Addr
	}
	s.Proc = proc.StartTCP(tb, proc.Options{
		Path: filepath.Join(s.Bin, "dreaderd"),
		Args: []string{"-d", "-p", PathHost, "-h", PathHost, "-P", bind},
		Env:  s.Env,
		Dir:  s.Dir,
	}, s.Addr)
	return s
}

// Reader dials dreaderd from 127.0.0.1 (read and post in the default dreader.access).
func (s *Site) Reader(tb testing.TB) *client.Conn {
	tb.Helper()
	return client.DialWith(tb, s.Addr, testkit.Deadline())
}

// spoolStore returns a Store with group e2e.test and a fake spool server serving it.
func spoolStore(tb testing.TB) (*fakeserver.Store, *fakeserver.Server) {
	tb.Helper()
	st := fakeserver.NewStore()
	st.AddGroup("e2e.test", "e2e test group")
	return st, fakeserver.New(tb, fakeserver.Options{Store: st})
}

// feedReader feeds articles (as built by newArticle) to the reader with IHAVE from PeerIP
// and stores them in the spool st, where dreaderd fetches the bodies.
func feedReader(tb testing.TB, s *Site, st *fakeserver.Store, group string, arts ...[]string) {
	tb.Helper()
	p := s.dialFrom(tb, PeerIP)
	for _, a := range arts {
		head, body := splitArticle(a)
		id := headerValue(head, "Message-ID")
		st.AddArticle(group, fakeserver.Article{MessageID: id, Headers: head, Body: body})
		expect(tb, p, 335, "IHAVE %s", id)
		send(tb, p, wire(a))
		status(tb, p, 235, "IHAVE "+id)
	}
	expect(tb, p, 205, "QUIT")
}

// TestReaderFeedAndRead feeds an article into dreaderd and reads it back: GROUP, OVER,
// HEAD, BODY and ARTICLE (the body fetched from the spool server).
func TestReaderFeedAndRead(t *testing.T) {
	st, spool := spoolStore(t)
	s := NewReaderSite(t, ReaderOptions{Spool: spool})
	id := msgID(t)
	feedReader(t, s, st, "e2e.test", newArticle(id, "e2e.test", "reader test", "hello reader", ".dot line"))

	c := s.Reader(t)
	if code, _ := client.ParseCode(c.Greeting); code != 200 {
		t.Fatalf("reader greeting %q, want 200", c.Greeting)
	}
	waitFor(t, testkit.Deadline(), "the article in GROUP e2e.test", func() bool {
		_, line := c.Cmd("GROUP e2e.test")
		f := strings.Fields(line)
		return len(f) > 1 && f[0] == "211" && f[1] != "0"
	})
	expect(t, c, 224, "OVER 1")
	over := c.ReadDotLines()
	if len(over) != 1 || !strings.Contains(over[0], id) {
		t.Fatalf("OVER 1: %q, want one line with %s", over, id)
	}
	expect(t, c, 222, "BODY %s", id)
	if got := c.ReadDotLines(); len(got) != 2 || got[0] != "hello reader" || got[1] != ".dot line" {
		t.Errorf("BODY: %q", got)
	}
	expect(t, c, 220, "ARTICLE 1")
	if got := c.ReadDotLines(); !containsLine(got, "Message-ID: "+id) {
		t.Errorf("ARTICLE 1: no Message-ID %s in %q", id, got)
	}
	expect(t, c, 205, "QUIT")
}

// TestReaderQuitAlone: QUIT on an idle dreaderd connection gets 205 and the connection closes
// with nothing after the 205.
func TestReaderQuitAlone(t *testing.T) {
	_, spool := spoolStore(t)
	s := NewReaderSite(t, ReaderOptions{Spool: spool})
	for i := range 3 {
		c := s.Reader(t)
		expect(t, c, 205, "QUIT")
		if err := c.ExpectNoPending(500 * time.Millisecond); err != nil {
			t.Errorf("connection %d: after QUIT's 205: %v", i+1, err)
		}
	}
}

// TestReaderPost: an article POSTed to dreaderd is passed to the outgoing ('o') server with
// IHAVE: the peer gets it with the body as posted and the vserver's postpath in the Path.
func TestReaderPost(t *testing.T) {
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
	// postpath: dreaderd adds this to the Path of local posts (without it the Path stays as
	// posted).
	access := strings.Replace(DefaultReaderAccess, "    hostname "+PathHost+"\n",
		"    hostname "+PathHost+"\n    postpath "+PathHost+"\n", 1)
	s := NewReaderSite(t, ReaderOptions{Spool: spool, Post: post, Access: access})
	c := s.Reader(t)
	id := msgID(t)
	expect(t, c, 340, "POST")
	send(t, c, wire([]string{
		"From: E2E Poster <poster@example.invalid>",
		"Newsgroups: e2e.test",
		"Subject: posted through dreaderd",
		"Message-ID: " + id,
		"",
		"posted body",
		".dot line",
	}))
	status(t, c, 240, "POST article")
	waitFor(t, 15*time.Second, "the outgoing server to receive the post", func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(arts) > 0
	})
	mu.Lock()
	got := slices.Clone(arts[0])
	mu.Unlock()
	head, body := splitArticle(got)
	if v := headerValue(head, "Message-ID"); v != id {
		t.Errorf("Message-ID at the peer %q, want %q", v, id)
	}
	if p := headerValue(head, "Path"); !strings.Contains(p, PathHost) {
		t.Errorf("Path at the peer %q, want it to name %s", p, PathHost)
	}
	// wire lines are dot-stuffed as sent
	if want := []string{"posted body", "..dot line"}; !slices.Equal(body, want) {
		t.Errorf("body at the peer %q, want %q", body, want)
	}
}

// TestReaderQuitWhileConnecting: a client that QUITs while another client is connecting gets
// 205 and nothing after it. With one DNS worker (readerdns 1), the end-of-session accounting of
// the first connection finds the worker busy with the second; dreaderd then writes "400 Server
// too busy" to the connection that already got its 205 (dreaderd/main.c SendDns).
func TestReaderQuitWhileConnecting(t *testing.T) {
	_, spool := spoolStore(t)
	s := NewReaderSite(t, ReaderOptions{Spool: spool})
	prev := s.Reader(t)
	for i := range 20 {
		next := make(chan *client.Conn, 1)
		go func() {
			c, err := client.TryDial(s.Addr, testkit.Deadline())
			if err != nil {
				t.Errorf("round %d: dial: %v", i+1, err)
			}
			next <- c
		}()
		expect(t, prev, 205, "QUIT")
		if err := prev.ExpectNoPending(300 * time.Millisecond); err != nil {
			t.Errorf("round %d: after QUIT's 205: %v", i+1, err)
		}
		prev.Close()
		prev = <-next
		if prev == nil {
			return
		}
		prev = prev.WithTB(t)
		t.Cleanup(prev.Close)
	}
	expect(t, prev, 205, "QUIT")
}

// TestHeaderFeedToReader: diablo feeds dreaderd a header-only feed (dnewsfeeds headfeed,
// dnewslink -H), and dreaderd fetches the body from diablo as its spool server: GROUP, OVER and
// BODY on the reader show the article fed into diablo.
func TestHeaderFeedToReader(t *testing.T) {
	const label = "reader"
	feeds := GlobalLabel + `label peer
    inhost ` + PeerIP + `
    alias e2e.peer
    addgroup *
end
label ` + label + `
    inhost 127.0.0.1
    hostname 127.0.0.1
    alias dummy
    headfeed on
    addgroup *
end
`
	feeder := NewSite(t, SiteOptions{Newsfeeds: feeds})
	r := NewReaderSite(t, ReaderOptions{SpoolAddr: feeder.Addr})

	id := msgID(t)
	p := feeder.Peer(t)
	expect(t, p, 335, "IHAVE %s", id)
	send(t, p, wire(newArticle(id, "e2e.test", "header feed", "body from the spool", ".dot")))
	status(t, p, 235, "IHAVE "+id)
	expect(t, p, 205, "QUIT")
	feeder.waitQueue(t, label, 1)

	_, rport, _ := net.SplitHostPort(r.Addr)
	// from PeerIP: the feed address in the reader's dreader.access
	feeder.Run(t, "dnewslink", "-b", label, "-h", "127.0.0.1", "-P", rport, "-H", "-B", PeerIP)

	c := r.Reader(t)
	waitFor(t, testkit.Deadline(), "the article in the reader's GROUP e2e.test", func() bool {
		_, line := c.Cmd("GROUP e2e.test")
		f := strings.Fields(line)
		return len(f) > 1 && f[0] == "211" && f[1] == "1"
	})
	expect(t, c, 224, "OVER 1")
	if over := c.ReadDotLines(); len(over) != 1 || !strings.Contains(over[0], id) {
		t.Fatalf("OVER 1: %q, want one line with %s", over, id)
	}
	expect(t, c, 222, "BODY %s", id)
	if got := c.ReadDotLines(); len(got) != 2 || got[0] != "body from the spool" || got[1] != ".dot" {
		t.Errorf("BODY (fetched from diablo): %q", got)
	}
	expect(t, c, 205, "QUIT")
}

// TestReaderBindHostPort: dreaderd -P 127.0.0.1:port listens there (before the fix it passed
// "127.0.0.1:port" as the host to getaddrinfo and exited).
func TestReaderBindHostPort(t *testing.T) {
	_, spool := spoolStore(t)
	s := NewReaderSite(t, ReaderOptions{Spool: spool, PlainBind: true})
	c := s.Reader(t)
	if code, _ := client.ParseCode(c.Greeting); code != 200 {
		t.Fatalf("greeting %q, want 200", c.Greeting)
	}
	expect(t, c, 205, "QUIT")
}

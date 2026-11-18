package e2e

// D-243 (deep review round 2): a request that waits in the list of unassigned server
// requests when its client goes away (idle timeout: NNTerminate sets its sr_CConn to NULL) was
// later matched against a server with groupdef=: dreaderd/server.c cbQueueReqFindThread read the
// client's group list through the NULL pointer and the reader process stopped, and the clients
// it served lost their connections.

import (
	"io"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	testkit "github.com/go-while/nntp-testkit"
	"github.com/go-while/nntp-testkit/client"
	"github.com/go-while/nntp-testkit/fakeserver"
)

// queuedGoneAccess: 127.0.0.1 has idletimeout 5 (the client that goes away, and a third one that
// keeps talking), 127.0.0.3 has none (the client whose request waits behind).
const queuedGoneAccess = `vserverdef DEFAULT
    hostname ` + PathHost + `
end
groupdef testonly
    group e2e.test
end
readerdef rp
    read yes
    post yes
    groups testonly
    idletimeout 5
end
readerdef wait
    read yes
    post yes
    groups testonly
end
readerdef feed
    feed yes
end
access 127.0.0.1 rp
access 127.0.0.3 wait
access ` + PeerIP + ` feed
`

// TestReaderQueuedRequestClientGone: the only matching server (groupdef=*) is down; client
// "gone" (idletimeout 5) sends a request and is closed by the idle timeout while it waits;
// client "waiter" (no idle timeout, from 127.0.0.3) sends one after it. When the server comes
// back, the reader process must still serve a third client, the waiter must get its answer, and
// the request of the client that went away must not reach the server (it is dropped): "article",
// the spool server, ARTICLE by Message-ID, gets no command line with its Message-ID; "post", the
// outgoing ('o') server, POST, gets no article with its Message-ID. Fails before the fix: the
// reader process stops ("Lost reader process") and the remaining clients lose their connection.
//
// What this test cannot tell apart: with only the NULL test in cbQueueReqFindThread (a request
// without a client matches no server) and no drop in QueueServerRequests, the request would
// stay in the list for good and only delay the requests behind it by one pass; the answers seen
// here would be the same.
func TestReaderQueuedRequestClientGone(t *testing.T) {
	t.Run("article", queuedGoneArticle)
	t.Run("post", queuedGonePost)
}

// reservedPort holds a TCP port for a server that is "down": a socket bound to it but not
// listening, so no other test or process can bind the port meanwhile. A connect to it is
// refused on Linux (as from a closed port); NetBSD drops the SYN instead, so the connect stays
// pending and completes on a retransmitted SYN once up has run (or times out and is tried
// again). up listens on that same socket, so the port is never free.
type reservedPort struct {
	fd   int
	addr string
	port int
}

// reservePort binds a socket to a free port of ip (an IPv4 address of 127/8) without listening.
func reservePort(t *testing.T, ip string) *reservedPort {
	t.Helper()
	// close-on-exec, so that the daemons the test starts do not inherit it (as Go does for
	// its own sockets, under ForkLock)
	syscall.ForkLock.RLock()
	fd, err := syscall.Socket(syscall.AF_INET, syscall.SOCK_STREAM, 0)
	if err == nil {
		syscall.CloseOnExec(fd)
	}
	syscall.ForkLock.RUnlock()
	if err != nil {
		t.Fatalf("socket: %v", err)
	}
	sa := &syscall.SockaddrInet4{}
	copy(sa.Addr[:], net.ParseIP(ip).To4())
	if err := syscall.Bind(fd, sa); err != nil {
		syscall.Close(fd)
		t.Fatalf("bind %s:0: %v", ip, err)
	}
	got, err := syscall.Getsockname(fd)
	if err != nil {
		syscall.Close(fd)
		t.Fatalf("getsockname: %v", err)
	}
	r := &reservedPort{fd: fd, port: got.(*syscall.SockaddrInet4).Port}
	r.addr = net.JoinHostPort(ip, strconv.Itoa(r.port))
	t.Cleanup(func() {
		if r.fd >= 0 {
			syscall.Close(r.fd)
		}
	})
	return r
}

// up starts listening on the reserved port and passes every connection through to target (a
// fake server on a port of its own), until the test ends.
func (r *reservedPort) up(t *testing.T, target string) {
	t.Helper()
	if err := syscall.Listen(r.fd, 16); err != nil {
		t.Fatalf("listen on %s: %v", r.addr, err)
	}
	// f owns the descriptor now; net.FileListener works on a copy of it
	f := os.NewFile(uintptr(r.fd), "reserved "+r.addr)
	r.fd = -1
	ln, err := net.FileListener(f)
	f.Close()
	if err != nil {
		t.Fatalf("listener on %s: %v", r.addr, err)
	}
	var mu sync.Mutex
	var conns []net.Conn
	t.Cleanup(func() {
		ln.Close()
		mu.Lock()
		defer mu.Unlock()
		for _, c := range conns {
			c.Close()
		}
	})
	go func() {
		for {
			in, err := ln.Accept()
			if err != nil {
				return
			}
			out, err := net.DialTimeout("tcp", target, 10*time.Second)
			if err != nil {
				in.Close()
				continue
			}
			mu.Lock()
			conns = append(conns, in, out)
			mu.Unlock()
			go func() { io.Copy(out, in); out.Close() }()
			go func() { io.Copy(in, out); in.Close() }()
		}
	}()
}

// waitIdleClosed keeps other talking while it waits for the idle timeout to close a client
// (the reader looks at idle clients at its next full pass, every 30 s or so).
func waitIdleClosed(t *testing.T, s *Site, other *client.Conn) {
	t.Helper()
	deadline := time.Now().Add(60 * time.Second)
	for !strings.Contains(generalLog(s), "timeout idle") && time.Now().Before(deadline) {
		time.Sleep(2 * time.Second)
		other.ExpectCmd(111, "DATE")
	}
	if !strings.Contains(generalLog(s), "timeout idle") {
		t.Fatalf("the idle timeout did not close the waiting client within 60 s:\n%s", generalLog(s))
	}
}

// waitQueuedFirst waits until dreaderd has put the request for id into its list of waiting
// requests: with no server to take it, queueServerRequest logs "Unable to find any spools ...
// for request <id>". Only then may the second client ask, so that gone's request is first.
func waitQueuedFirst(t *testing.T, s *Site, id string) {
	t.Helper()
	testkit.WaitFor(t, 20*time.Second, "dreaderd to queue the request of "+id, func() bool {
		return strings.Contains(generalLog(s), "for request "+id)
	})
}

// waitAccepted waits until dreaderd has connected to the returned server, and keeps other
// talking meanwhile (other has idletimeout 5, and the reader's pass that connects to the server
// also closes idle clients). dreaderd connects again at a later pass of its own, or, where a
// connect to the reserved port was not refused but its SYN dropped (NetBSD), when a retransmitted
// SYN of the connect it already started arrives after up(): that can take a minute or more.
func waitAccepted(t *testing.T, back *fakeserver.Server, other *client.Conn) {
	t.Helper()
	deadline := time.Now().Add(150 * time.Second)
	for back.Accepted() == 0 && time.Now().Before(deadline) {
		time.Sleep(2 * time.Second)
		other.ExpectCmd(111, "DATE")
	}
	if back.Accepted() == 0 {
		t.Fatalf("dreaderd did not connect to the returned server within 150 s")
	}
}

func checkReaderAlive(t *testing.T, s *Site, other *client.Conn) {
	t.Helper()
	if code, line, err := other.TryCmd("DATE"); err != nil || code != 111 {
		t.Errorf("DATE on the other client after the server returned: %d %q %v (reader process stopped?)", code, line, err)
	}
	if strings.Contains(generalLog(s), "Lost reader process") {
		t.Errorf("a reader process stopped:\n%s", generalLog(s))
	}
}

func queuedGoneArticle(t *testing.T) {
	st := fakeserver.NewStore()
	st.AddGroup("e2e.test", "e2e test group")
	// the spool server is down when dreaderd starts and when the clients ask; its port stays
	// reserved meanwhile
	rp := reservePort(t, "127.0.0.1")
	addr := rp.addr
	port := strconv.Itoa(rp.port)

	stored := msgID(t)
	head, body := splitArticle(newArticle(stored, "e2e.test", "queued behind a gone client", "body line"))
	st.AddArticle("e2e.test", fakeserver.Article{MessageID: stored, Headers: head, Body: body})

	s := NewReaderSite(t, ReaderOptions{SpoolAddr: addr, Access: queuedGoneAccess, NoStart: true,
		Files: map[string]string{"dserver.hosts": "127.0.0.1\ts1p" + port + " groupdef=*\n"}})
	startReader(t, s, "[127.0.0.1]:"+strings.TrimPrefix(s.Addr, "127.0.0.1:"))

	other := s.Reader(t) // stays connected and active, served by the same reader process
	other.ExpectCmd(111, "DATE")
	gone := s.Reader(t)
	goneID := msgID(t)
	gone.MustSend("ARTICLE " + goneID + "\r\n")
	waitQueuedFirst(t, s, goneID)
	waiter := s.dialFrom(t, "127.0.0.3")
	waiter.MustSend("ARTICLE " + stored + "\r\n")
	waitIdleClosed(t, s, other)

	back := fakeserver.New(t, fakeserver.Options{Store: st})
	rp.up(t, back.Addr())
	waitAccepted(t, back, other)

	code, line, err := waiter.ReadStatus()
	if err != nil || code != 220 {
		t.Fatalf("ARTICLE %s of the waiting client after the spool server returned: %d %q %v (reader process stopped?)\n%s",
			stored, code, line, err, generalLog(s))
	}
	lines, err := waiter.TryReadDotLines()
	if err != nil {
		t.Fatalf("article of the waiting client: %v", err)
	}
	if !strings.Contains(strings.Join(lines, "\n"), "body line") {
		t.Errorf("article of the waiting client has no body line: %q", lines)
	}
	checkReaderAlive(t, s, other)
	for _, l := range back.Lines() {
		if strings.Contains(l, goneID) {
			t.Errorf("the spool server got the request of the client that went away: %q", l)
		}
	}
}

func queuedGonePost(t *testing.T) {
	_, spool := spoolStore(t)
	var mu sync.Mutex
	var got []string
	po := &fakeserver.PeerOptions{OnArticle: func(id string, wire []string) {
		mu.Lock()
		defer mu.Unlock()
		got = append(got, id)
	}}
	// the outgoing server is down when dreaderd starts and when the clients post; its port
	// stays reserved meanwhile
	rp := reservePort(t, "127.0.0.2")
	pport := strconv.Itoa(rp.port)

	hosts := "127.0.0.1\ts1p" + strconv.Itoa(spool.Port()) + "\n127.0.0.2\to1p" + pport + " groupdef=*\n"
	s := NewReaderSite(t, ReaderOptions{Spool: spool, Access: queuedGoneAccess, NoStart: true,
		Files: map[string]string{"dserver.hosts": hosts}})
	startReader(t, s, "[127.0.0.1]:"+strings.TrimPrefix(s.Addr, "127.0.0.1:"))

	art := func(id string) string {
		return client.Stuff([]string{"From: E2E Poster <poster@example.invalid>", "Newsgroups: e2e.test",
			"Subject: posted while the outgoing server is down", "Message-ID: " + id, "", "posted body"})
	}
	other := s.Reader(t)
	other.ExpectCmd(111, "DATE")
	gone := s.Reader(t)
	goneID := msgID(t)
	gone.ExpectCmd(340, "POST")
	gone.MustSend(art(goneID))
	waitQueuedFirst(t, s, goneID)
	waiter := s.dialFrom(t, "127.0.0.3")
	waitID := msgID(t)
	waiter.ExpectCmd(340, "POST")
	waiter.MustSend(art(waitID))
	waitIdleClosed(t, s, other)

	back := fakeserver.New(t, fakeserver.Options{Addr: "127.0.0.2:0", Handler: fakeserver.PeerHandler(po)})
	rp.up(t, back.Addr())
	waitAccepted(t, back, other)

	code, line, err := waiter.ReadStatus()
	if err != nil || code != 240 {
		t.Fatalf("POST of the waiting client after the outgoing server returned: %d %q %v (reader process stopped?)\n%s",
			code, line, err, generalLog(s))
	}
	testkit.WaitFor(t, 15*time.Second, "the outgoing server to receive the waiting client's post", func() bool {
		mu.Lock()
		defer mu.Unlock()
		for _, id := range got {
			if id == waitID {
				return true
			}
		}
		return false
	})
	checkReaderAlive(t, s, other)
	mu.Lock()
	defer mu.Unlock()
	for _, id := range got {
		if id == goneID {
			t.Errorf("the outgoing server got the post of the client that went away (%s)", id)
		}
	}
}

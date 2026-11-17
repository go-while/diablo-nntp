package e2e

// D-243 (deep review round 2, M16): a request that waits in the list of unassigned server
// requests when its client goes away (idle timeout: NNTerminate sets its sr_CConn to NULL) was
// later matched against a spool server with groupdef=: dreaderd/server.c cbQueueReqFindThread
// read the client's group list through the NULL pointer and the reader process stopped, with
// every client it served.

import (
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/go-while/nntp-testkit/fakeserver"
)

// TestReaderQueuedRequestClientGone: the only spool server (groupdef=*) is down; client "gone"
// (readerdef with idletimeout 5) asks for an article by Message-ID and is closed by the idle
// timeout while its request waits; client "waiter" (no idle timeout, from 127.0.0.3) asks for a
// stored article after it. When the spool server comes back, the reader process must still
// serve its other client, and the waiter must get its article (the request of the client that
// went away is dropped, the one queued behind it is not). Fails before the fix: the reader
// process stops ("Lost reader process") and both remaining clients lose their connection.
func TestReaderQueuedRequestClientGone(t *testing.T) {
	st := fakeserver.NewStore()
	st.AddGroup("e2e.test", "e2e test group")
	spool := fakeserver.New(t, fakeserver.Options{Store: st})
	addr := spool.Addr()
	port := strconv.Itoa(spool.Port())
	spool.Close() // the server is down when dreaderd starts and when the clients ask

	stored := msgID(t)
	head, body := splitArticle(newArticle(stored, "e2e.test", "queued behind a gone client", "body line"))
	st.AddArticle("e2e.test", fakeserver.Article{MessageID: stored, Headers: head, Body: body})

	access := `vserverdef DEFAULT
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
    groups testonly
end
readerdef feed
    feed yes
end
access 127.0.0.1 rp
access 127.0.0.3 wait
access ` + PeerIP + ` feed
`
	s := NewReaderSite(t, ReaderOptions{SpoolAddr: addr, Access: access, NoStart: true,
		Files: map[string]string{"dserver.hosts": "127.0.0.1\ts1p" + port + " groupdef=*\n"}})
	startReader(t, s, "[127.0.0.1]:"+strings.TrimPrefix(s.Addr, "127.0.0.1:"))

	other := s.Reader(t) // stays connected and active, served by the same reader process
	other.ExpectCmd(111, "DATE")
	gone := s.Reader(t)
	gone.MustSend("ARTICLE " + msgID(t) + "\r\n")
	time.Sleep(500 * time.Millisecond) // gone's request is first in the list
	waiter := s.dialFrom(t, "127.0.0.3")
	waiter.MustSend("ARTICLE " + stored + "\r\n")

	// Both requests wait (no spool server to give them to). gone says nothing and is closed by
	// the idle timeout at the reader's next full pass (every 30 s or so).
	deadline := time.Now().Add(60 * time.Second)
	for !strings.Contains(generalLog(s), "timeout idle") && time.Now().Before(deadline) {
		time.Sleep(2 * time.Second)
		other.ExpectCmd(111, "DATE")
	}
	if !strings.Contains(generalLog(s), "timeout idle") {
		t.Fatalf("the idle timeout did not close the waiting client within 60 s:\n%s", generalLog(s))
	}

	back := fakeserver.New(t, fakeserver.Options{Addr: addr, Store: st})
	deadline = time.Now().Add(90 * time.Second)
	for back.Accepted() == 0 && time.Now().Before(deadline) {
		time.Sleep(500 * time.Millisecond)
	}
	if back.Accepted() == 0 {
		t.Fatalf("dreaderd did not connect to the returned spool server within 90 s")
	}

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
	code, line, err = other.TryCmd("DATE")
	if err != nil || code != 111 {
		t.Errorf("DATE on the other client after the spool server returned: %d %q %v (reader process stopped?)", code, line, err)
	}
	if strings.Contains(generalLog(s), "Lost reader process") {
		t.Errorf("a reader process stopped:\n%s", generalLog(s))
	}
}

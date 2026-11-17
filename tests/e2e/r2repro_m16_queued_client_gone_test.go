package e2e

// Deep review round 2, M16 (reproduction): a request that waits in the list of unassigned server
// requests when its client goes away is later tested against a spool server with groupdef=:
// dreaderd/server.c cbQueueReqFindThread reads sr_CConn, which NNTerminate set to NULL.

import (
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/go-while/nntp-testkit/fakeserver"
)

// TestR2ReproQueuedRequestClientGone: the only spool server (groupdef=testonly) is down; a client
// asks for an article by Message-ID and disconnects; the server comes back. The reader process
// must still serve its other client.
func TestR2ReproQueuedRequestClientGone(t *testing.T) {
	st := fakeserver.NewStore()
	st.AddGroup("e2e.test", "e2e test group")
	spool := fakeserver.New(t, fakeserver.Options{Store: st})
	addr := spool.Addr()
	port := strconv.Itoa(spool.Port())
	spool.Close() // the server is down when dreaderd starts and when the client asks

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
end
readerdef feed
    feed yes
end
access 127.0.0.1 rp
access ` + PeerIP + ` feed
`
	s := NewReaderSite(t, ReaderOptions{SpoolAddr: addr, Access: access, NoStart: true,
		Files: map[string]string{"dserver.hosts": "127.0.0.1\ts1p" + port + " groupdef=*\n"}})
	startReader(t, s, "[127.0.0.1]:"+strings.TrimPrefix(s.Addr, "127.0.0.1:"))

	other := s.Reader(t) // stays connected: it is served by the same reader process
	other.ExpectCmd(111, "DATE")
	gone := s.Reader(t)
	id := msgID(t)
	gone.MustSend("ARTICLE " + id + "\r\n")
	time.Sleep(2 * time.Second) // the request is queued: no spool server to give it to
	gone.Close()
	time.Sleep(3 * time.Second) // dreaderd sees the disconnect
	t.Logf("general.log before the spool server returns:\n%s", generalLog(s))

	back := fakeserver.New(t, fakeserver.Options{Addr: addr, Store: st})
	deadline := time.Now().Add(90 * time.Second)
	for back.Accepted() == 0 && time.Now().Before(deadline) {
		time.Sleep(500 * time.Millisecond)
	}
	if back.Accepted() == 0 {
		t.Fatalf("dreaderd did not connect to the returned spool server within 90 s")
	}
	time.Sleep(2 * time.Second)
	code, line, err := other.TryCmd("DATE")
	if err != nil || code != 111 {
		t.Errorf("DATE on the other client after the spool server returned: %d %q %v: the reader process died?", code, line, err)
	}
	t.Logf("general.log at the end:\n%s", generalLog(s))
}

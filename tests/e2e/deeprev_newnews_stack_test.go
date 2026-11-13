package e2e

import (
	"bufio"
	"fmt"
	"net"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-while/nntp-testkit/proc"
)

// TestReaderNewnewsManyMissingGroupsSmallStack: NEWNEWS took the next group of its list from
// inside the call that found nothing in the group before it (NNListNewNews() ->
// NNStartListOverviewRange() -> NNListNewNews() ...), one nested call pair per group that does
// not exist or holds no article in range. A line of about 110,000 names of groups that do not
// exist (under CLIENT_MAXLINE, 1 MB) nested that deep: the reader process, which serves all its
// clients, ran out of stack and died with SIGSEGV, and every client connected to it lost its
// connection. TestReaderNewnewsManyPlainNames hit it on NetBSD, whose default stack limit is
// 4 MB; Linux has 8 MB and got through that line by luck of the frame size.
//
// dreaderd runs with a stack limit of 256 KB here (the reader inherits it), well below what the
// nested calls of 110,000 names need on any system; the line must be answered with "230", the
// list (empty) and the ".", and another client must still be served afterwards.
func TestReaderNewnewsManyMissingGroupsSmallStack(t *testing.T) {
	const nNames = 110000
	access := strings.Replace(DefaultReaderAccess, "readerdef rp\n", "readerdef rp\n    allownewnews yes\n", 1)
	_, spool := spoolStore(t)
	s := NewReaderSite(t, ReaderOptions{Spool: spool, Access: access, Groups: []string{"comp.lang.c"}, NoStart: true})
	// The limit is set by a shell that then runs dreaderd, so it applies to dreaderd (and the
	// reader it forks) and to nothing else: not to this test process, and so not to the
	// programs other tests of this process start. (The test used to lower the limit of the
	// process itself around the start and ignored the result of putting it back.) If the shell
	// cannot set the limit it exits, and the start below fails.
	port := s.Addr[strings.LastIndex(s.Addr, ":")+1:]
	s.Proc = startOnListener(t, listenOn(t, s.Addr), proc.Options{
		Path: "/bin/sh",
		Args: []string{"-c", `ulimit -s 256 || exit 125; exec "$@"`, "e2e-smallstack",
			filepath.Join(s.Bin, "dreaderd"), "-d", "-p", PathHost, "-h", PathHost, "-P", "[127.0.0.1]:" + port,
			"-b", listenFDArg},
		Env: s.Env,
		Dir: s.Dir,
	}, s.Addr)

	var sb strings.Builder
	sb.WriteString("NEWNEWS x0000000")
	for i := 1; i < nNames; i++ {
		fmt.Fprintf(&sb, ",x%07d", i)
	}
	sb.WriteString(" 000101 000000 GMT\r\n")
	if sb.Len() > 1<<20 {
		t.Fatalf("the NEWNEWS line is %d bytes, more than CLIENT_MAXLINE", sb.Len())
	}

	dial := func() (net.Conn, *bufio.Reader) {
		nc, err := net.DialTimeout("tcp", s.Addr, 10*time.Second)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { nc.Close() })
		r := bufio.NewReaderSize(nc, 1<<16)
		nc.SetReadDeadline(time.Now().Add(20 * time.Second))
		if l, err := r.ReadString('\n'); err != nil || !strings.HasPrefix(l, "20") {
			t.Fatalf("greeting %q %v", l, err)
		}
		return nc, r
	}
	other, otherR := dial()
	nn, nnR := dial()

	nn.SetDeadline(time.Now().Add(60 * time.Second))
	if _, err := nn.Write([]byte(sb.String())); err != nil {
		t.Fatalf("writing the NEWNEWS line (%d bytes): %v", sb.Len(), err)
	}
	if l, err := nnR.ReadString('\n'); err != nil || !strings.HasPrefix(l, "230 ") {
		t.Fatalf("NEWNEWS status %q %v: the reader process died on the list of missing groups?", l, err)
	}
	if l, err := nnR.ReadString('\n'); err != nil || l != ".\r\n" {
		t.Fatalf("NEWNEWS list of groups that do not exist: %q %v, want the end of the list", l, err)
	}
	other.SetDeadline(time.Now().Add(20 * time.Second))
	fmt.Fprintf(other, "DATE\r\n")
	if l, err := otherR.ReadString('\n'); err != nil || !strings.HasPrefix(l, "111 ") {
		t.Fatalf("other client's DATE after the NEWNEWS: %q %v", l, err)
	}
}

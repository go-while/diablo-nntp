package e2e

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/go-while/nntp-testkit/proc"
)

// TestReaderFileDenyListFdLeak (D-168): a deny-list file authdef ("file !path")
// must fclose() the file when it finds the user, or every rejected login leaks
// one descriptor in the long-lived DNS worker.  With a 64-descriptor limit the
// worker then cannot fopen() the list and refuses every later login.
//
// The test rejects the listed user 100 times, then requires a user that is not
// in the deny list (and therefore allowed) to authenticate.  Base leaks and
// fails the last login; with the fix it succeeds.
//
// Linux also counts the descriptors in flight in AF_UNIX messages per user and refuses a
// pass once they exceed the sender's descriptor limit, here 64. When other processes of
// the same user (parallel test runs) hold that many in flight, dreaderd refuses clients
// with "too many descriptors in flight"; that says nothing about the leak, so the test
// skips instead of failing.
func TestReaderFileDenyListFdLeak(t *testing.T) {
	deny := filepath.Join(t.TempDir(), "deny")
	if err := os.WriteFile(deny, []byte("fred:pw\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	access := `vserverdef DEFAULT
    hostname ` + PathHost + `
end
authdef byfile
    file !` + deny + `
    user alice
    pass anything
end
readerdef rf
    read yes
    auth byfile
end
access 127.0.0.1 rf
`
	_, spool := spoolStore(t)
	s := NewReaderSite(t, ReaderOptions{Spool: spool, Access: access, NoStart: true})
	_, port, err := net.SplitHostPort(s.Addr)
	if err != nil {
		t.Fatal(err)
	}
	s.Proc = startSiteTCP(t, proc.Options{
		Path: filepath.Join(s.Bin, "dreaderd"),
		Args: []string{"-d", "-p", PathHost, "-h", PathHost,
			"-P", "[127.0.0.1]:" + port},
		Env:    s.Env,
		Dir:    s.Dir,
		Ulimit: []string{"-n 64"},
	}, s.Addr)

	skipIfInflight := func() {
		t.Helper()
		if strings.Contains(s.Proc.Stderr(), "descriptors in flight") {
			t.Skip("this user has too many descriptors in flight (other processes of the same user); the 64-descriptor limit cannot be tested here")
		}
	}
	login := func(user, pass string) (int, string) {
		t.Helper()
		nc, err := net.DialTimeout("tcp", s.Addr, 5*time.Second)
		if err != nil {
			t.Fatal(err)
		}
		defer nc.Close()
		nc.SetDeadline(time.Now().Add(10 * time.Second))
		r := bufio.NewReader(nc)
		cmd := func(line string) string {
			t.Helper()
			if line != "" {
				fmt.Fprintf(nc, "%s\r\n", line)
			}
			reply, err := r.ReadString('\n')
			if err != nil && reply == "" {
				skipIfInflight()
				t.Fatalf("%q: no reply: %v", line, err)
			}
			return strings.TrimRight(reply, "\r\n")
		}
		if g := cmd(""); !strings.HasPrefix(g, "20") {
			skipIfInflight()
			t.Fatalf("greeting: %q", g)
		}
		if l := cmd("AUTHINFO USER " + user); !strings.HasPrefix(l, "381") {
			t.Fatalf("AUTHINFO USER %s: %q", user, l)
		}
		l := cmd("AUTHINFO PASS " + pass)
		code, _ := strconv.Atoi(l[:min(3, len(l))])
		fmt.Fprintf(nc, "QUIT\r\n")
		return code, l
	}

	const n = 100
	for i := 0; i < n; i++ {
		if code, line := login("fred", "pw"); code == 281 {
			t.Fatalf("login %d: the denied user was accepted: %s", i, line)
		}
	}

	// A user that is not in the deny list is allowed.  If the worker leaked
	// a FILE per rejection it can no longer open the list here, and this
	// login is refused as well.
	// Only a refused connection (400) can come from the in-flight limit; an
	// authentication answer (481, 502) after the 100 rejections is the leak.
	if code, line := login("alice", "anything"); code != 281 {
		if code == 400 {
			skipIfInflight()
		}
		t.Errorf("the allowed user was refused after %d rejections: %s", n, line)
	}
}

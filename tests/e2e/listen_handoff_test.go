package e2e

// A daemon of a site gets its listening socket from the test instead of binding a port
// itself. testkit.FreePort finds a free port by listening on port 0 and closing that
// listener; the daemon bound the port only later, after the site's files were written and
// dgrpctl had run. In that gap the kernel could hand the same port to another site (of a
// parallel test, or of another process of ci/run-shards), and the daemon that bound it
// second exited with "bind: Address already in use" while proc.StartTCP took the greeting
// of the other site's daemon for its own: the test then ran against the other site
// ("430 No such article" for its own article, or no dhosts.cache).
//
// listenFree opens the listening socket at once and keeps it open; startOnListener hands
// it to the daemon (diablo -b and dreaderd -b serve an inherited listening descriptor) and
// closes the test's copy. The port is never free between the choice and the daemon, so
// nothing else can take it, and nothing but the site's daemon can answer on it.
//
// proc starts a program without extra descriptors, so a copy of the test binary
// (TestMain, runListenHandoff) is started in the daemon's place: it fetches the socket
// over a unix-domain socket (SCM_RIGHTS), puts its descriptor number in the daemon's
// arguments where they say listenFDArg, and execs the daemon, which keeps the pid, the
// process group and the output pipes that proc set up.

import (
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	testkit "github.com/go-while/nntp-testkit"
	"github.com/go-while/nntp-testkit/proc"
)

// listenHandoffEnv holds the unix-domain socket a handoff process fetches the listening
// socket from; with it set the test binary is that process (TestMain).
const listenHandoffEnv = "E2E_LISTEN_HANDOFF"

// listenFDArg in a daemon's arguments is replaced with the number of the descriptor of
// the listening socket, e.g. "-b", listenFDArg.
const listenFDArg = "@listenfd@"

// listenFree opens a listening socket on a free port of 127.0.0.1 for a site's daemon. The
// caller hands it to the daemon with startOnListener, or closes it.
func listenFree(tb testing.TB) *net.TCPListener {
	tb.Helper()
	for range 100 {
		port := testkit.FreePort(tb) // also skips a port in use on 0.0.0.0 or [::]
		l, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
		if errors.Is(err, syscall.EADDRINUSE) {
			continue // taken since FreePort closed it; nothing has been started on it
		}
		if err != nil {
			tb.Fatalf("listen: %v", err)
		}
		tb.Cleanup(func() { l.Close() }) // when the test fails before the daemon has it
		return l.(*net.TCPListener)
	}
	tb.Fatalf("listen: no free port of 127.0.0.1 in 100 tries")
	return nil
}

// listenOn opens a listening socket on addr, the address of a site whose daemon is not
// running (written with NoStart, or stopped to be started again).
func listenOn(tb testing.TB, addr string) *net.TCPListener {
	tb.Helper()
	l, err := net.Listen("tcp", addr)
	if err != nil {
		tb.Fatalf("listen on the site's address %s: %v", addr, err)
	}
	return l.(*net.TCPListener)
}

// startOnListener starts the daemon of o with startSiteTCP and hands it l, a listening
// socket on addr: listenFDArg in o.Args becomes its descriptor (diablo and dreaderd: "-b",
// listenFDArg). o.Path may be a wrapper (/bin/sh -c '... exec "$@"', taskset) that execs
// the daemon; the descriptor stays open across exec. l is closed when the daemon has it.
func startOnListener(tb testing.TB, l *net.TCPListener, o proc.Options, addr string) *proc.Process {
	tb.Helper()
	defer l.Close()
	if !slices.Contains(o.Args, listenFDArg) {
		tb.Fatalf("startOnListener: no %s in the arguments %q", listenFDArg, o.Args)
	}
	self, err := os.Executable()
	if err != nil {
		tb.Fatal(err)
	}
	sock := filepath.Join(testkit.ShortTempDir(tb), "l")
	u, err := net.ListenUnix("unix", &net.UnixAddr{Name: sock, Net: "unix"})
	if err != nil {
		tb.Fatal(err)
	}
	defer u.Close()
	sent := make(chan error, 1)
	go func() { sent <- sendListener(u, l) }()

	o.Args = append([]string{o.Path}, o.Args...)
	o.Path = self
	o.Env = append(slices.Clone(o.Env), listenHandoffEnv+"="+sock)
	p := startSiteTCP(tb, o, addr)
	// The daemon greeted on l, so the handoff is done.
	if err := <-sent; err != nil {
		tb.Fatalf("hand the listening socket to %s: %v", filepath.Base(o.Args[0]), err)
	}
	return p
}

// sendListener sends l's descriptor to the first handoff process that connects to u, then
// closes l: the socket stays open in the message and then in the daemon, which alone
// serves it from then on (when it exits, a connection is refused again).
func sendListener(u *net.UnixListener, l *net.TCPListener) error {
	defer l.Close()
	if err := u.SetDeadline(time.Now().Add(testkit.Deadline())); err != nil {
		return err
	}
	c, err := u.AcceptUnix()
	if err != nil {
		return err
	}
	defer c.Close()
	rc, err := l.SyscallConn() // not File().Fd(): that would make the socket blocking
	if err != nil {
		return err
	}
	var werr error
	if err := rc.Control(func(fd uintptr) {
		_, _, werr = c.WriteMsgUnix([]byte{0}, syscall.UnixRights(int(fd)), nil)
	}); err != nil {
		return err
	}
	return werr
}

// runListenHandoff makes the test binary the handoff process (see startOnListener) when
// listenHandoffEnv is set: started as "test-binary daemon args...", it fetches the listening
// socket and execs the daemon, or exits 125. Without listenHandoffEnv it returns.
func runListenHandoff() {
	sock := os.Getenv(listenHandoffEnv)
	if sock == "" {
		return
	}
	err := execOnListener(sock, os.Args[1:])
	fmt.Fprintf(os.Stderr, "e2e listen handoff: %v\n", err)
	os.Exit(125)
}

func execOnListener(sock string, args []string) error {
	if len(args) == 0 {
		return errors.New("no program to run")
	}
	c, err := net.DialTimeout("unix", sock, testkit.Deadline())
	if err != nil {
		return err
	}
	uc := c.(*net.UnixConn)
	if err := uc.SetDeadline(time.Now().Add(testkit.Deadline())); err != nil {
		return err
	}
	oob := make([]byte, syscall.CmsgSpace(4))
	_, oobn, _, _, err := uc.ReadMsgUnix(make([]byte, 1), oob)
	if err != nil {
		return fmt.Errorf("receive the listening socket: %w", err)
	}
	msgs, err := syscall.ParseSocketControlMessage(oob[:oobn])
	if err != nil || len(msgs) != 1 {
		return fmt.Errorf("receive the listening socket: %d control messages, %v", len(msgs), err)
	}
	fds, err := syscall.ParseUnixRights(&msgs[0])
	if err != nil || len(fds) != 1 {
		return fmt.Errorf("receive the listening socket: %d descriptors, %v", len(fds), err)
	}
	// The received descriptor may be close-on-exec; a dup of it is not.
	fd, err := syscall.Dup(fds[0])
	if err != nil {
		return err
	}
	syscall.Close(fds[0])

	path := args[0]
	if !strings.ContainsRune(path, filepath.Separator) {
		if path, err = exec.LookPath(path); err != nil {
			return err
		}
	}
	argv := slices.Clone(args)
	for i, a := range argv {
		if a == listenFDArg {
			argv[i] = strconv.Itoa(fd)
		}
	}
	env := slices.DeleteFunc(os.Environ(), func(e string) bool {
		return strings.HasPrefix(e, listenHandoffEnv+"=")
	})
	return syscall.Exec(path, argv, env)
}

// TestSitesOwnTheirPorts: reader sites started at the same time each answer on their own
// port: every site's vserver has its own hostname, and the greeting on its address must
// name it. Before listenFree two sites could be given one port, and the greeting on the
// second came from the first site's dreaderd (the second exited with "bind: Address already
// in use"). E2E_PORT_STRESS=n starts n sites instead of 16, to look for that race.
func TestSitesOwnTheirPorts(t *testing.T) {
	n := 16
	if v := os.Getenv("E2E_PORT_STRESS"); v != "" {
		var err error
		if n, err = strconv.Atoi(v); err != nil || n < 1 {
			t.Fatalf("E2E_PORT_STRESS=%q: want a number of sites", v)
		}
	}
	for i := range n {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			t.Parallel()
			_, spool := spoolStore(t)
			host := fmt.Sprintf("s%d.%s", i, PathHost)
			access := strings.Replace(DefaultReaderAccess, "hostname "+PathHost, "hostname "+host, 1)
			s := NewReaderSite(t, ReaderOptions{Spool: spool, Access: access})
			c := s.Reader(t)
			if f := strings.Fields(c.Greeting); len(f) < 2 || f[1] != host {
				t.Fatalf("greeting on %s: %q, want one from %s (this site's dreaderd)", s.Addr, c.Greeting, host)
			}
			c.ExpectCmd(205, "QUIT")
		})
	}
}

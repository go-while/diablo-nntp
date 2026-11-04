package e2e

// dreaderd's IPC between the main process and its readers with the socketpairs of illumos,
// on Linux. The shim (testdata/streamipc) refuses SOCK_SEQPACKET, so dreaderd falls back to
// SOCK_STREAM, and keeps the AF_UNIX buffers small (8 KiB asked, 16 KiB on Linux), as on
// illumos, which ignores the 256 KiB dreaderd asks for.

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

// startStreamIPCReader starts a reader site (AuthReaderAccess, readerthreads 64, plus conf)
// with the stream IPC shim, and checks that the shim did refuse SOCK_SEQPACKET.
func startStreamIPCReader(t *testing.T, conf map[string]string) *Site {
	t.Helper()
	if !preloadHost(false) {
		t.Skip("the stream IPC shim needs LD_PRELOAD (Linux, FreeBSD, NetBSD)")
	}
	cc := wmCC(t)
	dir := t.TempDir()
	so := filepath.Join(dir, "streamipc.so")
	src, err := filepath.Abs(filepath.Join("testdata", "streamipc", "streamipc.c"))
	if err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command(cc, append([]string{"-O2", "-shared", "-fPIC", "-o", so, src}, shimLibs()...)...).CombinedOutput(); err != nil {
		t.Skipf("cannot build the shim: %v\n%s", err, out)
	}
	marks := filepath.Join(dir, "refused.log")
	config := map[string]string{"readerthreads": "64"}
	for k, v := range conf {
		config[k] = v
	}
	_, spool := spoolStore(t)
	s := NewReaderSite(t, ReaderOptions{Spool: spool, Access: AuthReaderAccess,
		Config: config, NoStart: true})
	s.Env = append(s.Env, "LD_PRELOAD="+so, "STREAMIPC_BUF=8192", "STREAMIPC_LOG="+marks)
	_, port, _ := net.SplitHostPort(s.Addr)
	startReader(t, s, "[127.0.0.1]:"+port)
	if b, err := os.ReadFile(marks); err != nil || len(b) == 0 {
		t.Fatalf("the shim refused no SOCK_SEQPACKET socketpair (%v): the test would not use the stream fallback", err)
	}
	return s
}

// procChildren returns the child processes of pid, lowest pid first (dreaderd forks its
// readers first, then the feed fork and the DNS workers).
func procChildren(t *testing.T, pid int) []int {
	t.Helper()
	var kids []int
	if runtime.GOOS != "linux" {
		// no /proc here: ask ps (pid and parent pid of every process)
		out, err := exec.Command("ps", "-ax", "-o", "pid=", "-o", "ppid=").Output()
		if err != nil {
			t.Fatalf("ps: %v", err)
		}
		for _, l := range strings.Split(string(out), "\n") {
			if f := strings.Fields(l); len(f) == 2 && f[1] == strconv.Itoa(pid) {
				if p, err := strconv.Atoi(f[0]); err == nil {
					kids = append(kids, p)
				}
			}
		}
		sort.Ints(kids)
		return kids
	}
	ents, err := os.ReadDir("/proc")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range ents {
		p, err := strconv.Atoi(e.Name())
		if err != nil {
			continue
		}
		st, err := os.ReadFile(filepath.Join("/proc", e.Name(), "status"))
		if err != nil {
			continue
		}
		for _, l := range strings.Split(string(st), "\n") {
			if f := strings.Fields(l); len(f) == 2 && f[0] == "PPid:" && f[1] == strconv.Itoa(pid) {
				kids = append(kids, p)
			}
		}
	}
	sort.Ints(kids)
	return kids
}

type ipcConn struct {
	c  net.Conn
	rd *bufio.Reader
}

func (z *ipcConn) line(d time.Duration) string {
	z.c.SetReadDeadline(time.Now().Add(d))
	l, err := z.rd.ReadString('\n')
	if err != nil {
		return "error: " + err.Error()
	}
	return strings.TrimSpace(l)
}

// dialGreet opens n connections at once and reads each greeting (or an error text).
func dialGreet(addr string, n int, d time.Duration) ([]*ipcConn, []string) {
	cs := make([]*ipcConn, n)
	gs := make([]string, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			c, err := net.DialTimeout("tcp", addr, 10*time.Second)
			if err != nil {
				gs[i] = "error: " + err.Error()
				return
			}
			cs[i] = &ipcConn{c, bufio.NewReader(c)}
			gs[i] = cs[i].line(d)
		}(i)
	}
	wg.Wait()
	return cs, gs
}

func greeted(g string) bool {
	return strings.HasPrefix(g, "200") || strings.HasPrefix(g, "201")
}

// TestReaderDNSQueueStreamIPC: TestReaderDNSQueueLimit with the stream IPC shim. The 40
// hand-backs of the burst fill the reader-to-main direction while the replies fill the other
// one. Fails before the fix (the illumos CI stall): the main process waited in SendMsg() for
// the reader, which waited in SendMsg() for the main process, and the logins timed out.
func TestReaderDNSQueueStreamIPC(t *testing.T) {
	s := startStreamIPCReader(t, nil)
	for i, r := range authBurst(t, s.Addr, 40) {
		if !strings.HasPrefix(r, "281") {
			t.Errorf("login %d: got %q, want 281", i, r)
		}
	}
	if t.Failed() {
		return
	}
	c := s.Reader(t)
	c.ExpectCmd(381, "AUTHINFO USER fred")
	c.ExpectCmd(281, "AUTHINFO PASS fredspass")
	c.ExpectCmd(205, "QUIT")
}

// TestReaderStoppedDuringLoginBurst: the reader is stopped (SIGSTOP) for 2 s in the middle of
// the 40-login burst, while 20 more clients connect. Once it goes on, every login gets one
// 281, every QUIT one 205 and then EOF with nothing after it, and the late clients are
// greeted: the clients the main process queued for the reader were neither lost nor sent
// twice. Before the fix the main process waited for the reader in SendMsg() for the whole
// stop, and some runs never recovered.
func TestReaderStoppedDuringLoginBurst(t *testing.T) {
	s := startStreamIPCReader(t, nil)
	cs, gs := dialGreet(s.Addr, 40, 20*time.Second)
	for i, g := range gs {
		if !greeted(g) {
			t.Fatalf("client %d: greeting %q", i, g)
		}
		fmt.Fprintf(cs[i].c, "AUTHINFO USER fred\r\n")
		if r := cs[i].line(10 * time.Second); !strings.HasPrefix(r, "381") {
			t.Fatalf("client %d: AUTHINFO USER: %q", i, r)
		}
	}
	kids := procChildren(t, s.Proc.Pid())
	if len(kids) < 1 {
		t.Fatalf("no reader process found")
	}
	reader := kids[0]
	for i := range cs {
		fmt.Fprintf(cs[i].c, "AUTHINFO PASS fredspass\r\n")
	}
	time.Sleep(3 * time.Millisecond)
	if err := syscall.Kill(reader, syscall.SIGSTOP); err != nil {
		t.Fatal(err)
	}
	cont := func() { syscall.Kill(reader, syscall.SIGCONT) }
	defer cont()
	var late []*ipcConn
	var lateG []string
	var lw sync.WaitGroup
	lw.Add(1)
	go func() { defer lw.Done(); late, lateG = dialGreet(s.Addr, 20, 20*time.Second) }()
	time.Sleep(2 * time.Second)
	cont()
	for i := range cs {
		if r := cs[i].line(20 * time.Second); !strings.HasPrefix(r, "281") {
			t.Errorf("login %d: got %q, want 281", i, r)
		}
		fmt.Fprintf(cs[i].c, "QUIT\r\n")
		if r := cs[i].line(10 * time.Second); !strings.HasPrefix(r, "205") {
			t.Errorf("login %d: QUIT: got %q, want 205", i, r)
		}
		cs[i].c.SetReadDeadline(time.Now().Add(5 * time.Second))
		if extra, err := io.ReadAll(cs[i].rd); err != nil || len(extra) != 0 {
			t.Errorf("login %d: after 205: %q, %v; want EOF and nothing else", i, extra, err)
		}
		cs[i].c.Close()
	}
	lw.Wait()
	for i, g := range lateG {
		if !greeted(g) {
			t.Errorf("late client %d: greeting %q", i, g)
		}
		if late[i] != nil {
			late[i].c.Close()
		}
	}
	if t.Failed() {
		return
	}
	c := s.Reader(t)
	c.ExpectCmd(381, "AUTHINFO USER fred")
	c.ExpectCmd(281, "AUTHINFO PASS fredspass")
	c.ExpectCmd(205, "QUIT")
}

// TestReaderForkKeepsNoQueuedClients: with readerforks 2, one reader is stopped, so the main
// process queues the clients meant for it, and the other is killed, so the main process
// forks a new reader while that queue holds client descriptors. After the stopped reader
// goes on, every client gets 205 on QUIT and then EOF. Fails before the fix: the new reader
// kept a copy of each queued client socket, so those connections never closed.
func TestReaderForkKeepsNoQueuedClients(t *testing.T) {
	s := startStreamIPCReader(t, map[string]string{"readerforks": "2"})
	main := s.Proc.Pid()
	kids := procChildren(t, main)
	if len(kids) < 2 {
		t.Fatalf("children of the main process: %v, want 2 readers", kids)
	}
	initial := map[int]bool{}
	for _, p := range kids {
		initial[p] = true
	}
	stopped, other := kids[0], kids[1]
	if err := syscall.Kill(stopped, syscall.SIGSTOP); err != nil {
		t.Fatal(err)
	}
	cont := func() { syscall.Kill(stopped, syscall.SIGCONT) }
	defer cont()

	var cs []*ipcConn
	var gs []string
	var w sync.WaitGroup
	w.Add(1)
	go func() { defer w.Done(); cs, gs = dialGreet(s.Addr, 60, 30*time.Second) }()
	time.Sleep(1500 * time.Millisecond)
	if err := syscall.Kill(other, syscall.SIGKILL); err != nil {
		t.Fatal(err)
	}
	respawned := 0
	for deadline := time.Now().Add(20 * time.Second); respawned == 0 && time.Now().Before(deadline); {
		time.Sleep(100 * time.Millisecond)
		for _, p := range procChildren(t, main) {
			if !initial[p] {
				respawned = p
			}
		}
	}
	if respawned == 0 {
		t.Fatalf("the main process forked no new reader after the kill")
	}
	time.Sleep(300 * time.Millisecond)
	cont()
	w.Wait()

	greetedN, noEOF := 0, 0
	for i, g := range gs {
		if cs[i] == nil {
			continue
		}
		if !greeted(g) {
			// a client of the killed reader may get a 400; that is not what this tests
			cs[i].c.Close()
			continue
		}
		greetedN++
		fmt.Fprintf(cs[i].c, "QUIT\r\n")
		r := cs[i].line(10 * time.Second)
		cs[i].c.SetReadDeadline(time.Now().Add(3 * time.Second))
		if _, err := io.ReadAll(cs[i].rd); err != nil {
			noEOF++
			if noEOF <= 3 {
				t.Errorf("client %d: QUIT got %q, then no EOF within 3 s (%v)", i, r, err)
			}
		}
		cs[i].c.Close()
	}
	if noEOF > 0 {
		t.Errorf("%d of %d greeted clients got no EOF after QUIT: another process kept their socket open",
			noEOF, greetedN)
	}
	if greetedN < 30 {
		t.Errorf("only %d of 60 clients greeted", greetedN)
	}
}

// TestReaderQueueWaitLimit: with the only reader stopped, the clients the main process
// cannot pass to it wait at most readerdnswait (1 s here) and then get "400 Server too busy";
// the ones already in the reader's socket are greeted once it goes on, and nothing hangs.
func TestReaderQueueWaitLimit(t *testing.T) {
	s := startStreamIPCReader(t, map[string]string{"readerdnswait": "1"})
	kids := procChildren(t, s.Proc.Pid())
	if len(kids) < 1 {
		t.Fatalf("no reader process found")
	}
	reader := kids[0]
	if err := syscall.Kill(reader, syscall.SIGSTOP); err != nil {
		t.Fatal(err)
	}
	cont := func() { syscall.Kill(reader, syscall.SIGCONT) }
	defer cont()
	start := time.Now()
	var cs []*ipcConn
	var gs []string
	var w sync.WaitGroup
	w.Add(1)
	go func() { defer w.Done(); cs, gs = dialGreet(s.Addr, 40, 20*time.Second) }()
	time.Sleep(3500 * time.Millisecond)
	cont()
	w.Wait()
	busy, ok := 0, 0
	for i, g := range gs {
		switch {
		case greeted(g):
			ok++
		case strings.HasPrefix(g, "400") && strings.Contains(g, "too busy"):
			busy++
		default:
			t.Errorf("client %d: %q, want a greeting or 400 Server too busy", i, g)
		}
		if cs[i] != nil {
			cs[i].c.Close()
		}
	}
	t.Logf("%d greeted, %d refused, %v", ok, busy, time.Since(start))
	if busy == 0 {
		t.Errorf("no client was refused: the queue for the stopped reader has no time limit")
	}
	if ok == 0 {
		t.Errorf("no client was greeted after the reader went on")
	}
	c := s.Reader(t)
	c.ExpectCmd(381, "AUTHINFO USER fred")
	c.ExpectCmd(281, "AUTHINFO PASS fredspass")
	c.ExpectCmd(205, "QUIT")
}

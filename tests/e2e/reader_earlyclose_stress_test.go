package e2e

// A reader session must not end between the greeting and the client's first command.
// On NetBSD CI a reader once sent the 200 greeting and closed the session at once, in the
// same millisecond as another session on the same reader process ended with QUIT
// (TestNewgroupsLegacyDistributions, "read: connection reset by peer"). This test runs
// many short sessions in parallel against one reader process: half of the clients send
// QUIT at once, the other half GROUP or NEWGROUPS at once, so sessions end on the reader
// while new ones arrive. Any session that gets its greeting and then EOF or a reset
// before the reply fails the test.

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
)

// earlyCloseFail is one session that went wrong.
type earlyCloseFail struct {
	port  int    // client's local port
	mode  string // QUIT, GROUP or NEWGROUPS
	stage string // dial, greeting, reply, quit
	err   string
	eof   bool // EOF or reset, not a timeout or a wrong reply
	at    time.Time
}

func envIntDefault(name string, def int) int {
	if n, err := strconv.Atoi(os.Getenv(name)); err == nil && n > 0 {
		return n
	}
	return def
}

// earlyCloseSession runs one session: greeting, then cmd (if any), then QUIT. It returns
// the client's local port, the stage that failed ("" for none), and the error.
func earlyCloseSession(addr, mode, newgroupsArg string, d time.Duration) (int, string, error, bool) {
	conn, err := net.DialTimeout("tcp", addr, d)
	if err != nil {
		return 0, "dial", err, false
	}
	defer conn.Close()
	port := conn.LocalAddr().(*net.TCPAddr).Port
	conn.SetDeadline(time.Now().Add(d))
	r := bufio.NewReader(conn)
	line, err := r.ReadString('\n')
	if err != nil {
		return port, "greeting", err, false
	}
	if !strings.HasPrefix(line, "200 ") && !strings.HasPrefix(line, "201 ") {
		// a refusal (e.g. 400 limit reached) is no early close
		return port, "", nil, true
	}
	expect := func(cmd, code string) error {
		if _, err := conn.Write([]byte(cmd + "\r\n")); err != nil {
			return fmt.Errorf("write %q: %w", cmd, err)
		}
		l, err := r.ReadString('\n')
		if err != nil {
			return fmt.Errorf("reply to %q: %w (line %q)", cmd, err, l)
		}
		if !strings.HasPrefix(l, code+" ") {
			return fmt.Errorf("reply to %q: %q, want %s", cmd, strings.TrimSpace(l), code)
		}
		return nil
	}
	switch mode {
	case "GROUP":
		if err := expect("GROUP e2e.test", "211"); err != nil {
			return port, "reply", err, false
		}
	case "NEWGROUPS":
		if err := expect("NEWGROUPS "+newgroupsArg, "231"); err != nil {
			return port, "reply", err, false
		}
		for {
			l, err := r.ReadString('\n')
			if err != nil {
				return port, "reply", fmt.Errorf("NEWGROUPS list: %w", err), false
			}
			if l == ".\r\n" || l == ".\n" {
				break
			}
		}
	}
	stage := "quit"
	if mode == "QUIT" {
		stage = "reply" // QUIT is this session's first command
	}
	if err := expect("QUIT", "205"); err != nil {
		return port, stage, err, false
	}
	return port, "", nil, false
}

// isEarlyClose tells whether err is the symptom: the server ended the connection (EOF or
// reset) instead of answering.
func isEarlyClose(err error) bool {
	return errors.Is(err, io.EOF) || errors.Is(err, syscall.ECONNRESET) ||
		errors.Is(err, syscall.EPIPE) || errors.Is(err, io.ErrUnexpectedEOF)
}

func TestReaderEarlyCloseStress(t *testing.T) {
	for _, threads := range []string{"4", "64"} {
		t.Run("readerthreads"+threads, func(t *testing.T) {
			runEarlyCloseStress(t, threads)
		})
	}
}

func runEarlyCloseStress(t *testing.T, threads string) {
	sessions := envIntDefault("E2E_EARLYCLOSE_SESSIONS", 4000)
	workers := envIntDefault("E2E_EARLYCLOSE_WORKERS", 24)
	maxDur := time.Duration(envIntDefault("E2E_EARLYCLOSE_SECONDS", 60)) * time.Second
	_, spool := spoolStore(t)
	s := NewReaderSite(t, ReaderOptions{Spool: spool, Config: map[string]string{
		"readerforks":   "1",
		"readerthreads": threads,
		"readerdns":     "2",
	}})

	// On a failure, keep only the part of general.log that matters (it has thousands of
	// sessions): registered after NewReaderSite, so it runs before its dump.
	var (
		mu    sync.Mutex
		fails []earlyCloseFail
	)
	t.Cleanup(func() {
		if !t.Failed() {
			return
		}
		mu.Lock()
		defer mu.Unlock()
		path := filepath.Join(s.Dir, "log", "general.log")
		b, err := os.ReadFile(path)
		if err != nil {
			return
		}
		os.WriteFile(path, []byte(earlyCloseExcerpt(string(b), fails)), 0o644)
	})

	ahead := time.Now().UTC().Add(time.Hour).Format("20060102 150405")
	var (
		next     atomic.Int64
		refused  atomic.Int64
		ok       atomic.Int64
		wg       sync.WaitGroup
		deadline = time.Now().Add(maxDur)
	)
	start := time.Now()
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i := 0; ; i++ {
				if next.Add(1) > int64(sessions) || time.Now().After(deadline) {
					return
				}
				mode := "QUIT"
				if w%2 == 1 {
					mode = []string{"GROUP", "NEWGROUPS"}[i%2]
				}
				port, stage, err, ref := earlyCloseSession(s.Addr, mode, ahead, 10*time.Second)
				if ref {
					refused.Add(1)
					continue
				}
				if stage == "" {
					ok.Add(1)
					continue
				}
				mu.Lock()
				fails = append(fails, earlyCloseFail{port, mode, stage, err.Error(), isEarlyClose(err), time.Now()})
				mu.Unlock()
			}
		}(w)
	}
	wg.Wait()
	el := time.Since(start)

	g := generalLog(s)
	diag := regexp.MustCompile(`(?m)^.*DIAG .*$`).FindAllString(g, -1)
	t.Logf("readerthreads %s: %d sessions ok, %d refused, %d failed in %v (%d workers); %d DIAG lines",
		threads, ok.Load(), refused.Load(), len(fails), el.Round(time.Millisecond), workers, len(diag))
	for i, l := range diag {
		if i == 40 {
			t.Logf("... %d more DIAG lines", len(diag)-40)
			break
		}
		t.Logf("diag: %s", l)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(fails) == 0 {
		return
	}
	byStage := map[string]int{}
	early := 0
	for _, f := range fails {
		byStage[f.stage]++
		if f.stage == "reply" && f.eof {
			early++
		}
	}
	keys := make([]string, 0, len(byStage))
	for k := range byStage {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var sum []string
	for _, k := range keys {
		sum = append(sum, fmt.Sprintf("%s=%d", k, byStage[k]))
	}
	for i, f := range fails {
		if i == 20 {
			t.Logf("... %d more failed sessions", len(fails)-20)
			break
		}
		t.Logf("failed: %s port %d mode %s stage %s eof %v: %s", f.at.Format("15:04:05.000000"), f.port, f.mode, f.stage, f.eof, f.err)
	}
	t.Errorf("%d of %d sessions failed (%s); %d ended after the greeting before the first reply",
		len(fails), int64(len(fails))+ok.Load()+refused.Load(), strings.Join(sum, " "), early)
}

// earlyCloseExcerpt returns the DIAG lines of a general.log and the lines around those that
// name a failed session's port.
func earlyCloseExcerpt(g string, fails []earlyCloseFail) string {
	lines := strings.Split(g, "\n")
	keep := make([]bool, len(lines))
	for i, l := range lines {
		if strings.Contains(l, "DIAG ") {
			keep[i] = true
		}
	}
	for n, f := range fails {
		if n == 20 {
			break
		}
		p := ":" + strconv.Itoa(f.port)
		for i, l := range lines {
			if strings.Contains(l, p+")") || strings.Contains(l, p+" ") || strings.HasSuffix(l, p) {
				for j := max(0, i-12); j < min(len(lines), i+13); j++ {
					keep[j] = true
				}
			}
		}
	}
	var b strings.Builder
	fmt.Fprintf(&b, "[excerpt of %d lines: DIAG lines and +-12 lines around failed ports]\n", len(lines))
	last := -2
	for i, l := range lines {
		if !keep[i] {
			continue
		}
		if i != last+1 {
			b.WriteString("...\n")
		}
		b.WriteString(l + "\n")
		last = i
	}
	return b.String()
}

package e2e

import (
	"bytes"
	"context"
	"fmt"
	"maps"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	testkit "github.com/go-while/nntp-testkit"
	"github.com/go-while/nntp-testkit/client"
	"github.com/go-while/nntp-testkit/proc"
)

// EnvBin names the environment variable that holds the directory with the Diablo programs
// under test (diablo, dicmd, dnewslink, ...), e.g. the dbin directory of a build. Without it
// every test is skipped.
const EnvBin = "DIABLO_BIN"

// PeerIP is the loopback address the default dnewsfeeds lists as the inhost of the label
// "peer". A connection from 127.0.0.1 has no label and is refused.
const PeerIP = "127.0.0.2"

// PathHost is the site's pathhost.
const PathHost = "e2e.test"

// SiteOptions configures a throwaway Diablo feeder site. In every file, @home@ is replaced
// with the site's home directory and @bindir@ with DIABLO_BIN.
type SiteOptions struct {
	Config    map[string]string // diablo.config directives added or replacing the generated ones
	Newsfeeds string            // dnewsfeeds; "" = DefaultNewsfeeds
	SpoolCtl  string            // dspool.ctl; "" = DefaultSpoolCtl
	Files     map[string]string // further files in the home directory, by name
	Args      []string          // further diablo arguments, before "server"

	// Setup runs after the files are written and before diablo starts, e.g. to change a
	// file's mode.
	Setup func(tb testing.TB, s *Site)
}

// Site is one running diablo server with its own path_home in a temporary directory.
type Site struct {
	Dir  string   // path_home: diablo.config, dnewsfeeds, dhistory, spool, dqueue, log, run
	Bin  string   // DIABLO_BIN
	Addr string   // diablo's listening address, 127.0.0.1:port
	Env  []string // DIABLO_CONFIG_PATH=Dir/diablo.config, for Diablo programs run by the test
	Proc *proc.Process
}

// bin returns DIABLO_BIN or skips the test.
func bin(tb testing.TB) string {
	tb.Helper()
	b := os.Getenv(EnvBin)
	if b == "" {
		tb.Skipf("%s is not set: point it at the dbin directory of a Diablo build", EnvBin)
	}
	if _, err := os.Stat(filepath.Join(b, "diablo")); err != nil {
		tb.Fatalf("%s=%s: %v", EnvBin, b, err)
	}
	return b
}

// GlobalLabel is the GLOBAL label of dnewsfeeds; every other label refers to it, and diablo
// logs "grouplist GLOBAL does not exist" for each one when it is missing.
const GlobalLabel = "label GLOBAL\nend\n"

// DefaultNewsfeeds lets PeerIP feed every group under the label "peer".
const DefaultNewsfeeds = GlobalLabel + `label peer
    inhost ` + PeerIP + `
    addgroup *
end
`

// DefaultSpoolCtl stores every article in spool 00.
const DefaultSpoolCtl = `spool 00
    minfree 1m
end
metaspool trad
    spool 00
end
expire * trad
`

// NewSite writes a feeder site into a temporary directory, starts diablo -d (in the
// foreground) on a free port of 127.0.0.1 and waits until it accepts connections and has
// built its host cache. The cleanup stops diablo with "dicmd exit", then proc's cleanup
// sends SIGTERM and SIGKILL if it is still running, and logs diablo's output when the test
// failed.
func NewSite(tb testing.TB, o SiteOptions) *Site {
	tb.Helper()
	s := &Site{Dir: tb.TempDir(), Bin: bin(tb)}
	port := freePort(tb)
	s.Addr = net.JoinHostPort("127.0.0.1", strconv.Itoa(port))
	s.Env = []string{"DIABLO_CONFIG_PATH=" + filepath.Join(s.Dir, "diablo.config")}

	for _, d := range []string{"spool/news", "dqueue", "feeds", "log", "run"} {
		if err := os.MkdirAll(filepath.Join(s.Dir, d), 0o755); err != nil {
			tb.Fatal(err)
		}
	}

	conf := map[string]string{
		"path_home":     s.Dir,
		"active":        "off",
		"hsize":         "256k", // the smallest diablo accepts
		"remember":      "7",
		"feederrtstats": "none",
	}
	for k, v := range o.Config {
		conf[k] = s.expand(v)
	}
	var b strings.Builder
	b.WriteString("version 2.0\n") // directives after it are read with the 2.0 grammar
	for _, k := range slices.Sorted(maps.Keys(conf)) {
		fmt.Fprintf(&b, "%s\t%s\n", k, conf[k])
	}
	s.WriteFile(tb, "diablo.config", b.String())
	s.WriteFile(tb, "dnewsfeeds", s.expand(or(o.Newsfeeds, DefaultNewsfeeds)))
	s.WriteFile(tb, "dspool.ctl", s.expand(or(o.SpoolCtl, DefaultSpoolCtl)))
	for name, content := range o.Files {
		s.WriteFile(tb, name, s.expand(content))
	}
	if o.Setup != nil {
		o.Setup(tb, s)
	}

	args := append([]string{"-d", "-p", PathHost, "-h", PathHost, "-P", s.Addr}, o.Args...)
	s.Proc = proc.StartTCP(tb, proc.Options{
		Path: filepath.Join(s.Bin, "diablo"),
		Args: append(args, "server"),
		Env:  s.Env,
		Dir:  s.Dir,
	}, s.Addr)
	tb.Cleanup(func() { s.shutdown(tb) }) // runs before proc's cleanup (SIGTERM, then SIGKILL)

	// diablo rebuilds dhosts.cache in a child process after it starts listening; until the
	// cache is there every peer is refused.
	waitFor(tb, testkit.Deadline(), "dhosts.cache", func() bool {
		_, err := os.Stat(filepath.Join(s.Dir, "dhosts.cache"))
		return err == nil
	})
	return s
}

// expand replaces @home@ and @bindir@ in a configuration file.
func (s *Site) expand(content string) string {
	return strings.NewReplacer("@home@", s.Dir, "@bindir@", s.Bin).Replace(content)
}

// shutdown stops a running diablo with "dicmd exit" and waits for it to exit.
func (s *Site) shutdown(tb testing.TB) {
	select {
	case <-s.Proc.Done():
		return
	default:
	}
	if out, err := s.run("dicmd", "exit"); err != nil {
		tb.Logf("dicmd exit: %v\n%s", err, out)
		return // proc's cleanup sends SIGTERM, then SIGKILL
	}
	select {
	case <-s.Proc.Done():
	case <-time.After(10 * time.Second):
		tb.Errorf("diablo still running 10s after dicmd exit")
	}
}

// Run runs the Diablo program name from DIABLO_BIN with the site's DIABLO_CONFIG_PATH in Dir
// and returns its combined output; a failure (non-zero exit, or still running after the hang
// guard) fails the test.
func (s *Site) Run(tb testing.TB, name string, args ...string) string {
	tb.Helper()
	out, err := s.run(name, args...)
	if err != nil {
		tb.Fatalf("%s %s: %v\n%s", name, strings.Join(args, " "), err, out)
	}
	return out
}

// TryRun is Run that returns the error instead of failing the test.
func (s *Site) TryRun(name string, args ...string) (string, error) { return s.run(name, args...) }

func (s *Site) run(name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), testkit.Deadline())
	defer cancel()
	cmd := exec.CommandContext(ctx, filepath.Join(s.Bin, name), args...)
	cmd.Dir = s.Dir
	cmd.Env = append(os.Environ(), s.Env...)
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	err := cmd.Run()
	return out.String(), err
}

// PrimeHostCache builds dhosts.cache with "dprimehostcache -prime", for a SiteOptions.Setup:
// diablo then starts with a cache, as on any start after the first.
func PrimeHostCache(tb testing.TB, s *Site) {
	tb.Helper()
	s.Run(tb, "dprimehostcache", "-prime")
}

// Dicmd runs dicmd with args and fails the test unless it succeeds.
func (s *Site) Dicmd(tb testing.TB, args ...string) string {
	tb.Helper()
	return s.Run(tb, "dicmd", args...)
}

// Peer dials diablo from PeerIP (the inhost of the label "peer" in the default dnewsfeeds)
// and reads the greeting.
func (s *Site) Peer(tb testing.TB) *client.Conn {
	tb.Helper()
	return s.dialFrom(tb, PeerIP)
}

// Stranger dials diablo from 127.0.0.1, which no label lists, and reads the greeting.
func (s *Site) Stranger(tb testing.TB) *client.Conn {
	tb.Helper()
	return s.dialFrom(tb, "127.0.0.1")
}

func (s *Site) dialFrom(tb testing.TB, ip string) *client.Conn {
	tb.Helper()
	d := net.Dialer{LocalAddr: &net.TCPAddr{IP: net.ParseIP(ip)}, Timeout: testkit.Deadline()}
	nc, err := d.Dial("tcp", s.Addr)
	if err != nil {
		tb.Fatalf("dial %s from %s: %v", s.Addr, ip, err)
	}
	c := client.New(tb, nc)
	g, err := c.ReadGreeting()
	if err != nil {
		tb.Fatalf("greeting from %s: %v", ip, err)
	}
	c.Greeting = g
	return c
}

// WriteFile writes a file of the site, relative to Dir.
func (s *Site) WriteFile(tb testing.TB, rel, content string) {
	tb.Helper()
	if err := os.WriteFile(filepath.Join(s.Dir, rel), []byte(content), 0o644); err != nil {
		tb.Fatal(err)
	}
}

// ReadFile returns a file of the site, relative to Dir.
func (s *Site) ReadFile(tb testing.TB, rel string) string {
	tb.Helper()
	b, err := os.ReadFile(filepath.Join(s.Dir, rel))
	if err != nil {
		tb.Fatal(err)
	}
	return string(b)
}

// freePort returns a TCP port of 127.0.0.1 that was free a moment ago.
func freePort(tb testing.TB) int {
	tb.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		tb.Fatal(err)
	}
	defer func() { _ = l.Close() }()
	return l.Addr().(*net.TCPAddr).Port
}

// waitFor polls cond every 50ms until it holds or d passes; then it fails the test with what.
func waitFor(tb testing.TB, d time.Duration, what string, cond func() bool) {
	tb.Helper()
	limit := time.Now().Add(d)
	for !cond() {
		if time.Now().After(limit) {
			tb.Fatalf("timed out after %v waiting for %s", d, what)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func or(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

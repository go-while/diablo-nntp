package e2e

import (
	"bytes"
	"github.com/go-while/nntp-testkit/fakeserver"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// D-69: a compression level zlib rejects (dnewsfeeds "compress 12" becomes dnewslink -Z12): the peer
// accepts MODE COMPRESS, deflateInit() fails, but compression stays switched on. The first
// write then spins in flushCompressBuffer() forever (deflate keeps failing, avail_in never
// drops), logging an error on every pass.
func TestDnewslinkBadCompressLevel(t *testing.T) {
	skipWithoutZlib(t)
	s := NewSite(t, SiteOptions{NoStart: true})
	dnlSpool(t, s)
	var checked atomic.Bool
	srv := fakeserver.New(t, fakeserver.Options{Greeting: "200 fake", Handler: fakeserver.LineHandler(func(_ *fakeserver.Conn, line string) ([]string, bool) {
		line = strings.ToLower(strings.TrimSpace(line))
		switch {
		case strings.HasPrefix(line, "mode stream"):
			return []string{"203 stream ok"}, false
		case strings.HasPrefix(line, "mode compress"):
			return []string{"207 compress ok"}, false
		case strings.HasPrefix(line, "check "):
			if strings.HasPrefix(line, "check <z1@") {
				checked.Store(true)
			}
			return []string{"438 " + strings.Fields(line)[1]}, false
		}
		return nil, false
	})})
	port := srv.Port()
	q := "D.00000001/B.0001 <z1@e2e.test> 0,100\n"
	if err := os.WriteFile(s.Dir+"/dqueue/out.S00000", []byte(q), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(filepath.Join(s.Bin, "dnewslink"), "-N", "1", "-S", "0", "-b", "out.S%05d",
		"-h", "127.0.0.1", "-P", strconv.Itoa(port), "-t", "3", "-Z", "12")
	cmd.Dir = s.Dir
	cmd.Env = append(os.Environ(), s.Env...)
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() { _ = cmd.Wait(); close(done) }()
	defer func() { _ = cmd.Process.Kill(); <-done }()
	select {
	case <-done:
		t.Logf("dnewslink ended by itself:\n%s", out.String())
		if !checked.Load() {
			t.Errorf("the peer got no CHECK <z1@e2e.test>: nothing was offered, so the compressed write was not tested")
		}
	case <-time.After(8 * time.Second):
		ps, _ := exec.Command("ps", "-o", "pid,time,pcpu,args", "-p", strconv.Itoa(cmd.Process.Pid)).CombinedOutput()
		t.Errorf("dnewslink -Z12 still busy after 8 s against a peer that accepted MODE COMPRESS:\n%s\noutput so far:\n%s", ps, out.String())
	}
}

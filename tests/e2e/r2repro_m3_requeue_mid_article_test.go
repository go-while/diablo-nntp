package e2e

// Deep review round 2, M3 (reproduction): a spool request that is requeued after the spool server
// was closed on the T timeout (dreaderd/spool.c, server.c NNServerTerminate) while part of the
// article had already gone to the client and into the cache.

import (
	"bufio"
	"net"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/go-while/nntp-testkit/fakeserver"
)

// r2Raw is a client that reads for as long as the test needs (a requeue takes up to 35 s).
type r2Raw struct {
	t *testing.T
	c net.Conn
	r *bufio.Reader
}

func r2Dial(t *testing.T, addr string) *r2Raw {
	t.Helper()
	c, err := net.DialTimeout("tcp", addr, 10*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Close() })
	x := &r2Raw{t: t, c: c, r: bufio.NewReader(c)}
	x.line(10 * time.Second) // greeting
	return x
}

func (x *r2Raw) line(d time.Duration) (string, error) {
	x.c.SetReadDeadline(time.Now().Add(d))
	s, err := x.r.ReadString('\n')
	return strings.TrimRight(s, "\r\n"), err
}

func (x *r2Raw) send(s string) {
	x.t.Helper()
	x.c.SetWriteDeadline(time.Now().Add(10 * time.Second))
	if _, err := x.c.Write([]byte(s)); err != nil {
		x.t.Fatalf("write %q: %v", s, err)
	}
}

// block reads lines up to the "." line (or the error) within d.
func (x *r2Raw) block(d time.Duration) (lines []string, err error) {
	end := time.Now().Add(d)
	for {
		l, e := x.line(time.Until(end))
		if e != nil {
			return lines, e
		}
		if l == "." {
			return lines, nil
		}
		lines = append(lines, l)
	}
}

func TestR2ReproRequeueMidArticle(t *testing.T) {
	for _, cache := range []bool{false, true} {
		name := "cache off"
		if cache {
			name = "cache on"
		}
		t.Run(name, func(t *testing.T) { r2RequeueMidArticle(t, cache) })
	}
}

// r2RequeueMidArticle: server 1 (priority 1, T5) sends the status line, the headers and one body
// line of an article and then says nothing until dreaderd closes the connection; server 2 has the
// article. The client must get one status line and the article once.
func r2RequeueMidArticle(t *testing.T, cache bool) {
	id := msgID(t)
	art := newArticle(id, "e2e.test", "requeue", "body 1", "body 2", "body 3", "body 4")
	head, _ := splitArticle(art)

	st := fakeserver.NewStore()
	st.AddGroup("e2e.test", "e2e test group")
	good := fakeserver.New(t, fakeserver.Options{Addr: "127.0.0.2:0", Store: st})
	stall := fakeserver.New(t, fakeserver.Options{Handler: func(c *fakeserver.Conn) {
		for {
			line, err := c.ReadLine()
			if err != nil {
				return
			}
			f := strings.Fields(line)
			switch {
			case len(f) == 2 && strings.EqualFold(f[0], "ARTICLE"):
				c.WriteLines("220 0 " + f[1] + " article follows")
				c.WriteLines(head...)
				c.WriteLines("", "stalled body line")
				for { // silent until dreaderd closes the connection
					if _, err := c.ReadLine(); err != nil {
						return
					}
				}
			case len(f) > 0 && strings.EqualFold(f[0], "QUIT"):
				c.WriteLines("205 bye")
				return
			default:
				c.WriteLines("500 what?")
			}
		}
	}})
	conf := map[string]string{"readercache": "off"}
	if cache {
		conf["readercache"] = "on"
	}
	hosts := "127.0.0.1\ts1p" + strconv.Itoa(stall.Port()) + " T5\n127.0.0.2\ts2p" + strconv.Itoa(good.Port()) + "\n"
	s := NewReaderSite(t, ReaderOptions{SpoolAddr: stall.Addr(), Config: conf,
		Files: map[string]string{"dserver.hosts": hosts}})
	feedReader(t, s, st, "e2e.test", art)

	x := r2Dial(t, s.Addr)
	start := time.Now()
	x.send("ARTICLE " + id + "\r\n")
	status, err := x.line(120 * time.Second)
	if err != nil {
		t.Fatalf("no status line for ARTICLE within 120 s: %v", err)
	}
	lines, err := x.block(120 * time.Second)
	t.Logf("after %v: status %q, %d lines, err %v", time.Since(start).Round(time.Second), status, len(lines), err)
	if !strings.HasPrefix(status, "220") {
		t.Fatalf("status %q, want 220", status)
	}
	for _, l := range lines {
		if strings.HasPrefix(l, "220 ") {
			t.Errorf("a second status line inside the article: %q", l)
		}
		if l == "stalled body line" {
			t.Errorf("the partial text of the stalled server reached the client: %q", lines)
		}
	}
	var body []string
	if i := slices.Index(lines, ""); i >= 0 {
		body = lines[i+1:]
	}
	want := []string{"body 1", "body 2", "body 3", "body 4"}
	if strings.Join(body, "|") != strings.Join(want, "|") {
		t.Errorf("body of the reply %q, want %q", body, want)
	}
	x.send("DATE\r\n")
	if l, err := x.line(10 * time.Second); err != nil || !strings.HasPrefix(l, "111") {
		t.Errorf("DATE after the article: %q %v (a stray line of the earlier reply?)", l, err)
	}
	if cache {
		time.Sleep(2 * time.Second)
		dir := filepath.Join(s.Dir, "spool", "cache")
		filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
			if err == nil && d.Type().IsRegular() {
				b, _ := os.ReadFile(p)
				t.Logf("cache file %s: %d bytes, %d status lines, stalled line: %v", strings.TrimPrefix(p, dir), len(b),
					strings.Count(string(b), "220 "), strings.Contains(string(b), "stalled body line"))
				if strings.Contains(string(b), "stalled body line") {
					t.Errorf("the cache file holds the partial text of the stalled server")
				}
			}
			return nil
		})
	}
	t.Logf("general.log:\n%s", generalLog(s))
}

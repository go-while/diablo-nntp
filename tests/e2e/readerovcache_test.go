package e2e

// readerovcache: how many groups' overview files each dreaderd process keeps open.

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	testkit "github.com/go-while/nntp-testkit"
	"github.com/go-while/nntp-testkit/fakeserver"
	"github.com/go-while/nntp-testkit/proc"
)

// TestReaderOvCacheConfig: readerovcache takes 2 to 65536 groups; anything else is refused
// and the value stays (drcmd config on a running dreaderd).
func TestReaderOvCacheConfig(t *testing.T) {
	_, spool := spoolStore(t)
	s := NewReaderSite(t, ReaderOptions{Spool: spool})
	if out := s.Run(t, "drcmd", "config", "readerovcache"); !strings.Contains(out, "readerovcache: 8") {
		t.Errorf("drcmd config readerovcache: %q, want the default 8", out)
	}
	if out := s.Run(t, "drcmd", "config", "readerovcache", "512"); !strings.Contains(out, "Set 'readerovcache' to '512'") {
		t.Errorf("drcmd config readerovcache 512: %q", out)
	}
	for _, bad := range []string{"1", "0", "65537", "-4", "abc", "64x"} {
		if out := s.Run(t, "drcmd", "config", "readerovcache", bad); strings.Contains(out, "Set '") {
			t.Errorf("drcmd config readerovcache %s: %q, want it refused", bad, out)
		}
	}
	if out := s.Run(t, "drcmd", "config", "readerovcache"); !strings.Contains(out, "readerovcache: 512") {
		t.Errorf("drcmd config readerovcache after refused values: %q, want 512", out)
	}
}

// TestReaderOvCacheSmall: with readerovcache 2 the reader takes and serves articles for six
// groups in turn, each one evicting the others' open overview files, and every group's
// overview is complete and in order.
func TestReaderOvCacheSmall(t *testing.T) {
	const ngroups, rounds = 6, 3
	st := fakeserver.NewStore()
	var groups []string
	for i := 1; i <= ngroups; i++ {
		g := fmt.Sprintf("e2e.cache%d", i)
		groups = append(groups, g)
		st.AddGroup(g, "cache test group")
	}
	spool := fakeserver.New(t, fakeserver.Options{Store: st})
	s := NewReaderSite(t, ReaderOptions{Spool: spool, Groups: groups,
		Config: map[string]string{"readerovcache": "2"}})
	ids := map[string][]string{}
	for r := 0; r < rounds; r++ {
		for _, g := range groups {
			id := msgID(t)
			ids[g] = append(ids[g], id)
			feedReader(t, s, st, g, newArticle(id, g, fmt.Sprintf("round %d", r+1), "body"))
		}
	}
	c := s.Reader(t)
	for _, g := range groups {
		testkit.WaitFor(t, testkit.Deadline(), "3 articles in "+g, func() bool {
			_, line := c.Cmd("GROUP %s", g)
			f := strings.Fields(line)
			return len(f) > 1 && f[0] == "211" && f[1] == fmt.Sprint(rounds)
		})
		c.ExpectCmd(224, "OVER 1-%d", rounds)
		over := c.ReadDotLines()
		if len(over) != rounds {
			t.Fatalf("OVER in %s: %q, want %d lines", g, over, rounds)
		}
		for i, l := range over {
			if f := strings.Split(l, "\t"); len(f) < 5 || f[4] != ids[g][i] {
				t.Errorf("OVER line %d in %s: %q, want message-id %s", i+1, g, l, ids[g][i])
			}
		}
	}
}

// TestReaderOvCacheLowered: a readerovcache that does not fit the descriptor limit is
// lowered when the first group is opened, and logged: with 128 descriptors and 4 threads,
// (128 - 4 - 64) / 2 = 30 groups. The reader still works.
func TestReaderOvCacheLowered(t *testing.T) {
	st, spool := spoolStore(t)
	s := NewReaderSite(t, ReaderOptions{Spool: spool, NoStart: true,
		Config: map[string]string{"readerovcache": "1000", "readerthreads": "4"}})
	port := strings.TrimPrefix(s.Addr, "127.0.0.1:")
	s.Proc = startSiteTCP(t, proc.Options{
		Path:   filepath.Join(s.Bin, "dreaderd"),
		Args:   []string{"-d", "-p", PathHost, "-h", PathHost, "-P", "[127.0.0.1]:" + port},
		Env:    s.Env,
		Dir:    s.Dir,
		Ulimit: []string{"-n 128"},
	}, s.Addr)
	id := msgID(t)
	feedReader(t, s, st, "e2e.test", newArticle(id, "e2e.test", "lowered cache", "body"))
	want := "readerovcache 1000 lowered to 30: descriptor limit 128, readerthreads 4"
	testkit.WaitFor(t, 10*time.Second, "the lowered readerovcache in the log", func() bool {
		return strings.Contains(generalLog(s), want)
	})
	c := s.Reader(t)
	testkit.WaitFor(t, testkit.Deadline(), "the article in GROUP e2e.test", func() bool {
		_, line := c.Cmd("GROUP e2e.test")
		f := strings.Fields(line)
		return len(f) > 1 && f[0] == "211" && f[1] == "1"
	})
	c.ExpectCmd(223, "STAT %s", id)
}

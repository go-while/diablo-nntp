package e2e

import (
	"strings"
	"testing"

	testkit "github.com/go-while/nntp-testkit"
)

// D-197: a vserverdef block without a hostname line started from an empty definition (the
// "vserverdef" keyword cleared the defaults dreaderd had set for the built-in DEFAULT), so
// the host name and the cluster name were empty: the banner read "200  NNRP Service Ready"
// and dreaderd added nothing to Path or Xref. Such a block now serves as the host given
// with -h (else the system's host name), its cluster name is that host, and its newsadmin
// defaults to dreaderd's (-A, else news@host). A block with its own hostname and no newsadmin
// line gets news@<its hostname>, as samples/dreader.access says ("Defaults to news@hostname").

const d197Readers = `readerdef rp
    read yes
    post yes
    vserver %s
end
readerdef feed
    feed yes
end
access 127.0.0.1 rp
access ` + PeerIP + ` feed
`

func d197Access(vserver, body string) string {
	return "vserverdef " + vserver + "\n" + body + "end\n" +
		strings.Replace(d197Readers, "%s", vserver, 1)
}

// TestVserverdefWithoutHostname: the banner names the -h host (and news@ that host) for a
// DEFAULT and a named vserverdef that set only postpath, and an article read through them
// gets that host in front of its Path. Fails on main: "200  NNRP Service Ready (posting
// ok)." and the Path as fed.
func TestVserverdefWithoutHostname(t *testing.T) {
	for _, vs := range []string{"DEFAULT", "local"} {
		t.Run(vs, func(t *testing.T) {
			st, spool := spoolStore(t)
			s := NewReaderSite(t, ReaderOptions{Spool: spool,
				Access: d197Access(vs, "    postpath "+PathHost+"\n")})
			want := "200 " + PathHost + " NNRP Service Ready - news@" + PathHost + " "
			if g := readerGreeting(s.Addr); !strings.HasPrefix(g, want) {
				t.Fatalf("greeting %q, want it to start with %q", strings.TrimSpace(g), want)
			}

			id := msgID(t)
			feedReader(t, s, st, "e2e.test", newArticle(id, "e2e.test", "d197", "body"))
			c := s.Reader(t)
			testkit.WaitFor(t, testkit.Deadline(), "the article in GROUP e2e.test", func() bool {
				_, line := c.Cmd("GROUP e2e.test")
				f := strings.Fields(line)
				return len(f) > 1 && f[0] == "211" && f[1] != "0"
			})
			c.ExpectCmd(221, "HEAD %s", id)
			head := c.ReadDotLines()
			var path string
			for _, l := range head {
				if strings.HasPrefix(l, "Path: ") {
					path = l
				}
			}
			if wantPath := "Path: " + PathHost + "!e2e.peer!not-for-mail"; path != wantPath {
				t.Errorf("HEAD %s: %q, want %q (the cluster name in front)", id, path, wantPath)
			}
		})
	}
}

// TestVserverdefOwnHostname: a vserverdef with its own hostname greets with that host; without
// a newsadmin line its newsadmin is news@<that host>, and its own newsadmin line is kept. The
// first case fails on main: no newsadmin ("200 own.example NNRP Service Ready (posting ok).").
func TestVserverdefOwnHostname(t *testing.T) {
	for _, c := range []struct{ name, body, want string }{
		{"default-newsadmin", "    hostname own.example\n",
			"200 own.example NNRP Service Ready - news@own.example (posting ok)."},
		{"own-newsadmin", "    hostname own.example\n    newsadmin admin@own.example\n",
			"200 own.example NNRP Service Ready - admin@own.example (posting ok)."},
	} {
		t.Run(c.name, func(t *testing.T) {
			_, spool := spoolStore(t)
			s := NewReaderSite(t, ReaderOptions{Spool: spool, Access: d197Access("own", c.body)})
			if g := strings.TrimSpace(readerGreeting(s.Addr)); g != c.want {
				t.Errorf("greeting %q, want %q", g, c.want)
			}
		})
	}
}

// TestVserverdefNewsadminNone: "newsadmin none" (any case) leaves the newsadmin empty, so the
// banner has no address: for a block with its own hostname and for one that takes the -h host.
// Fails on main: the default news@<host> is shown.
func TestVserverdefNewsadminNone(t *testing.T) {
	for _, c := range []struct{ name, body, want string }{
		{"own-host", "    hostname own.example\n    newsadmin none\n",
			"200 own.example NNRP Service Ready (posting ok)."},
		{"h-host", "    postpath " + PathHost + "\n    newsadmin NONE\n",
			"200 " + PathHost + " NNRP Service Ready (posting ok)."},
	} {
		t.Run(c.name, func(t *testing.T) {
			_, spool := spoolStore(t)
			s := NewReaderSite(t, ReaderOptions{Spool: spool, Access: d197Access("own", c.body)})
			if g := strings.TrimSpace(readerGreeting(s.Addr)); g != c.want {
				t.Errorf("greeting %q, want %q", g, c.want)
			}
		})
	}
}

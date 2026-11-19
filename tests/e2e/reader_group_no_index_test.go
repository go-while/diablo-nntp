package e2e

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	testkit "github.com/go-while/nntp-testkit"
	"github.com/go-while/nntp-testkit/client"
)

// TestReaderGroupNoIndexCreate: reader commands on a group that has no over.* overview
// index, and a cancel that names such a group, must not create the index. dreaderd's
// GetOverInfo() opened the index with O_CREAT and sized a new one (header plus 512 article
// slots, two fsync()), so GROUP, LISTGROUP, NEWNEWS (one command for all the groups its
// wildmat matches), overview and article commands and cancels left an index file behind
// for each group they looked at. The read and cancel paths now leave a missing index
// missing and take it as a group without articles; the feed creates it:
//   - e2e.empty, e2e.listgroup (no article yet, NB 1 NE 0): GROUP 211 0, LISTGROUP an
//     empty list.
//   - e2e.wide (NE set to 2000 by hand, no index): GROUP and LISTGROUP answer
//     211 1025 976 2000: the low number is max(NB, NE-1024), the bound dreaderd uses when
//     a group has no index (on main the index GROUP had just created gave NE-512 here);
//     XOVER 1500-2000 lists nothing, HEAD and ARTICLE 1990 are 423.
//   - e2e.nn (NB 1 NE 0): NEWNEWS e2e.nn lists nothing.
//   - e2e.cancel: the cancel of an article not here yet still counts the group as valid
//     (readercanlock all: the cancel is remembered and the article refused with 437).
//
// Last, an article fed into e2e.empty creates its index. The feed connection (from
// PeerIP, feed only) is served by the feed-only process, so the reader process that
// looked e2e.empty up before without an index serves the reader connection: a new GROUP
// e2e.empty there (WaitFor repeats it) shows the article and XOVER lists it.
func TestReaderGroupNoIndexCreate(t *testing.T) {
	st, spool := spoolStore(t)
	access := strings.Replace(DefaultReaderAccess, "readerdef rp\n", "readerdef rp\n    allownewnews yes\n", 1)
	s := NewReaderSite(t, ReaderOptions{
		Spool:   spool,
		Access:  access,
		Groups:  []string{"e2e.test", "e2e.empty", "e2e.listgroup", "e2e.wide", "e2e.nn", "e2e.cancel"},
		Config:  map[string]string{"readercanlock": "all"},
		Files:   map[string]string{"dcontrol.ctl": "cancel:*:*:doit,break\n"},
		NoStart: true,
	})
	s.Run(t, "dgrpctl", "adjust", "e2e.wide", "NE", "=2000")
	_, port, _ := strings.Cut(s.Addr, ":")
	startReader(t, s, "[127.0.0.1]:"+port)

	home := strings.TrimSpace(s.Run(t, "dpath", "group"))
	// index is the path of the group's over.* index (iteration 0).
	index := func(group string) string {
		f := strings.Fields(s.Run(t, "dgrpctl", "listgrouphashfile", group))
		if len(f) != 2 || !strings.HasPrefix(filepath.Base(f[0]), "over.0.") {
			t.Fatalf("dgrpctl listgrouphashfile %s: %q", group, f)
		}
		return filepath.Join(home, f[0])
	}
	exists := func(group string) bool {
		_, err := os.Stat(index(group))
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			t.Fatal(err)
		}
		return err == nil
	}
	noIndex := func(after, group string) {
		t.Helper()
		if exists(group) {
			t.Errorf("after %s the overview index of %s exists (%s): reader commands and cancels must not create it",
				after, group, index(group))
		}
	}
	for _, g := range []string{"e2e.empty", "e2e.listgroup", "e2e.wide", "e2e.nn", "e2e.cancel"} {
		if exists(g) {
			t.Fatalf("%s has an overview index before the test: %s", g, index(g))
		}
	}

	c := s.Reader(t)
	line := c.ExpectCmd(211, "GROUP e2e.empty")
	t.Logf("GROUP e2e.empty: %q", line)
	if f := strings.Fields(line); len(f) < 5 || f[1] != "0" || f[4] != "e2e.empty" {
		t.Errorf("GROUP e2e.empty: %q, want 211 0 <low> <high> e2e.empty", line)
	}
	noIndex("GROUP", "e2e.empty")

	line = c.ExpectCmd(211, "LISTGROUP e2e.listgroup")
	t.Logf("LISTGROUP e2e.listgroup: %q", line)
	if lines := c.ReadDotLines(); len(lines) != 0 {
		t.Errorf("LISTGROUP e2e.listgroup: %q, want an empty list", lines)
	}
	noIndex("LISTGROUP", "e2e.listgroup")

	// A group whose numbers say 1..2000 but that has no index.
	if line = c.ExpectCmd(211, "GROUP e2e.wide"); line != "211 1025 976 2000 e2e.wide" {
		t.Errorf("GROUP e2e.wide: %q, want \"211 1025 976 2000 e2e.wide\" (no index: low = NE-1024)", line)
	}
	c.ExpectCmd(224, "XOVER 1500-2000")
	if lines := c.ReadDotLines(); len(lines) != 0 {
		t.Errorf("XOVER 1500-2000 in e2e.wide: %q, want an empty list", lines)
	}
	for _, cmd := range []string{"HEAD 1990", "ARTICLE 1990"} {
		if code, line := c.Cmd("%s", cmd); code != 423 {
			t.Errorf("%s in e2e.wide: %q, want 423", cmd, line)
			if code/100 == 2 {
				c.ReadDotLines()
			}
		}
	}
	noIndex("GROUP, XOVER, HEAD and ARTICLE", "e2e.wide")
	if line = c.ExpectCmd(211, "LISTGROUP e2e.wide"); line != "211 1025 976 2000 e2e.wide list follows" {
		t.Errorf("LISTGROUP e2e.wide: %q, want \"211 1025 976 2000 e2e.wide list follows\"", line)
	}
	if lines := c.ReadDotLines(); len(lines) != 0 {
		t.Errorf("LISTGROUP e2e.wide: %q, want an empty list", lines)
	}
	noIndex("LISTGROUP", "e2e.wide")

	c.ExpectCmd(230, "NEWNEWS e2e.nn 19700101 000000 GMT")
	if lines := c.ReadDotLines(); len(lines) != 0 {
		t.Errorf("NEWNEWS e2e.nn: %q, want an empty list", lines)
	}
	noIndex("NEWNEWS", "e2e.nn")

	// A cancel for an article that is not here, in e2e.cancel. The cancel article itself is
	// filed under control.cancel, so the cancel's lookup is what opens e2e.cancel.
	target := msgID(t)
	feedAndSettle(t, s, st, c, cancelArticle(t, target, "e2e.cancel"))
	noIndex("a cancel naming the group", "e2e.cancel")
	// e2e.cancel still counted as a valid group of the cancel: the target is refused.
	p := s.dialFrom(t, PeerIP)
	p.ExpectCmd(335, "IHAVE %s", target)
	p.MustSend(client.Stuff(newArticle(target, "e2e.cancel", "cancelled before it came", "body")))
	if code, line, err := p.ReadStatus(); err != nil || code != 437 {
		t.Errorf("IHAVE of the cancelled article: %d %q %v, want 437 (in the cancel cache)", code, line, err)
	}
	p.ExpectCmd(205, "QUIT")
	noIndex("the refused article", "e2e.cancel")

	// The feed creates the index; a new GROUP on the reader connection shows the article.
	id := msgID(t)
	feedReader(t, s, st, "e2e.empty", newArticle(id, "e2e.empty", "first article", "hello"))
	testkit.WaitFor(t, testkit.Deadline(), "the article in GROUP e2e.empty", func() bool {
		_, line := c.Cmd("GROUP e2e.empty")
		f := strings.Fields(line)
		return len(f) >= 5 && f[0] == "211" && f[1] == "1"
	})
	c.ExpectCmd(224, "XOVER 1-10")
	if lines := c.ReadDotLines(); len(lines) != 1 || !strings.Contains(lines[0], id) {
		t.Errorf("XOVER 1-10 in e2e.empty after the feed: %q, want the article %s", lines, id)
	}
	if !exists("e2e.empty") {
		t.Errorf("the feed did not create the overview index of e2e.empty (%s)", index("e2e.empty"))
	}
}

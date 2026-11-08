package e2e

import (
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	testkit "github.com/go-while/nntp-testkit"
)

// TestDexpireoverRewriteGzipData: dexpireover -R on a group whose overview data file exists only
// as .gz (made by dexpireover -z). With zlib the data is unpacked into a plain file, bytes
// unchanged. A build with ./configure --without-zlib cannot read it: it used to delete the .gz
// and leave an empty data file with exit 0. Now it leaves the group's files alone, says that a
// zlib build is needed, and exits non-zero. Plain data is rewritten by both builds.
func TestDexpireoverRewriteGzipData(t *testing.T) {
	zlib := builtWithZlib(t)
	for _, compressed := range []bool{true, false} {
		name := "plain"
		if compressed {
			name = "gzip"
		}
		t.Run(name, func(t *testing.T) {
			s, home := overSite(t, map[string]string{"readergrouphash": "hierarchy"})
			s.Run(t, "dgrpctl", "adjust", "e2e.test", "NE", "=1")

			// article 1: one header block at offset 0 of d.0, ending in its NUL guard
			raw := []byte("Subject: overview data\r\nMessage-ID: <gz@example.com>\r\nX-Note: " +
				strings.Repeat("some header text ", 30) + "\r\n\x00")
			head := overHead("e2e.test")
			index := append(head, make([]byte, 1000*32)...)
			slot := index[len(head)+32:]
			o := binary.NativeEndian
			o.PutUint32(slot[0:], 1)
			o.PutUint32(slot[4:], 0)
			o.PutUint32(slot[8:], uint32(len(raw)-1))
			o.PutUint32(slot[12:], 0x952a1fae)
			o.PutUint32(slot[16:], 0x54dcbdde)
			o.PutUint32(slot[20:], 1000)
			o.PutUint32(slot[24:], uint32(time.Now().Unix()))
			over := filepath.Join(home, "e2e", "o.0.e2e.test")
			writeOver(t, over, index)

			data := filepath.Join(home, "e2e", "d.0.e2e.test")
			var gz bytes.Buffer
			if compressed {
				w := gzip.NewWriter(&gz)
				if _, err := w.Write(raw); err != nil {
					t.Fatal(err)
				}
				if err := w.Close(); err != nil {
					t.Fatal(err)
				}
				writeOver(t, data+".gz", gz.Bytes())
			} else {
				writeOver(t, data, raw)
			}

			out, err := s.TryRun("dexpireover", "-R", "-v")
			plain, plainErr := os.ReadFile(data)
			if compressed && !zlib {
				if err == nil {
					t.Errorf("dexpireover -R exits 0 on .gz data without zlib:\n%s", out)
				}
				if !strings.Contains(out, "needs a zlib build") {
					t.Errorf("no message that a zlib build is needed:\n%s", out)
				}
				if b, err := os.ReadFile(data + ".gz"); err != nil || !bytes.Equal(b, gz.Bytes()) {
					t.Errorf("the .gz data file changed or is gone (%v):\n%s", err, out)
				}
				if plainErr == nil {
					t.Errorf("a plain data file of %d bytes was made:\n%s", len(plain), out)
				}
				if b, err := os.ReadFile(over); err != nil || !bytes.Equal(b, index) {
					t.Errorf("the over file changed (%v):\n%s", err, out)
				}
				return
			}
			if err != nil {
				t.Errorf("dexpireover -R: %v\n%s", err, out)
			}
			if plainErr != nil || !bytes.Equal(plain, raw) {
				t.Errorf("data file after the rewrite: %d bytes, want the %d bytes of before (%v):\n%s",
					len(plain), len(raw), plainErr, out)
			}
			if _, err := os.Stat(data + ".gz"); err == nil {
				t.Errorf("the .gz data file is still there next to the plain one:\n%s", out)
			}
		})
	}
}

// TestReaderReadsGzipData: dreaderd serves OVER, HDR, XPAT, and HEAD and ARTICLE by number (the
// number is looked up in the overview) from overview data files that dexpireover -z gzipped.
// dreaderd unpacks such a file into a temporary file when it opens it
// (dreaderd/group.c, GetOverData); until now no test read through that path. With e4 and 12
// articles a group has four data files; F1 keeps the two newest plain and Z1 lets the two
// older ones (made two hours old) be compressed. After a dreaderd restart (so no open plain
// file is cached), the answers are the same as before, a header that is not an overview field
// included, and a new article still goes into a plain data file.
func TestReaderReadsGzipData(t *testing.T) {
	skipWithoutZlib(t)
	st, spool := spoolStore(t)
	s := NewReaderSite(t, ReaderOptions{Spool: spool,
		Files: map[string]string{"dexpire.ctl": "e2e.*:x30:e4:F1:Z1\n"}})

	article := func(n int) []string {
		a := newArticle(msgID(t), "e2e.test", fmt.Sprintf("gzip data %02d", n), "body")
		head, body := splitArticle(a)
		head = append(head, fmt.Sprintf("X-Gz-Note: note %02d", n))
		return append(append(head, ""), body...)
	}
	var arts [][]string
	for n := 1; n <= 12; n++ {
		arts = append(arts, article(n))
	}
	feedReader(t, s, st, "e2e.test", arts...)

	read := func() map[string][]string {
		c := s.Reader(t)
		testkit.WaitFor(t, 10*time.Second, "12 articles in GROUP e2e.test", func() bool {
			_, line := c.Cmd("GROUP e2e.test")
			return strings.HasPrefix(line, "211 12 1 12 ")
		})
		got := map[string][]string{}
		for _, cmd := range []struct {
			code int
			text string
		}{
			{224, "OVER 1-12"},
			{225, "HDR Subject 1-12"},
			{225, "HDR X-Gz-Note 1-12"},
			{221, "XPAT Subject 1-12 *data 0[3-6]"},
			{221, "HEAD 2"},
			{220, "ARTICLE 6"},
		} {
			c.ExpectCmd(cmd.code, "%s", cmd.text)
			got[cmd.text] = c.ReadDotLines()
		}
		c.ExpectCmd(205, "QUIT")
		return got
	}
	before := read()
	if n := len(before["HDR X-Gz-Note 1-12"]); n != 12 {
		t.Fatalf("HDR X-Gz-Note before compression: %d lines, want 12: %q", n, before["HDR X-Gz-Note 1-12"])
	}
	for _, cmd := range []string{"HEAD 2", "ARTICLE 6"} {
		if !slices.Contains(before[cmd], "X-Gz-Note: note 0"+cmd[len(cmd)-1:]) {
			t.Fatalf("%s before compression is not article %s: %q", cmd, cmd[len(cmd)-1:], before[cmd])
		}
	}
	if n := len(before["XPAT Subject 1-12 *data 0[3-6]"]); n != 4 {
		t.Fatalf("XPAT before compression: %d lines, want 4: %q", n, before["XPAT Subject 1-12 *data 0[3-6]"])
	}

	dataFiles := func() (plain, gz []string) {
		m, _ := filepath.Glob(filepath.Join(s.Dir, "spool", "group", "*", "data.*"))
		m2, _ := filepath.Glob(filepath.Join(s.Dir, "spool", "group", "data.*"))
		for _, f := range append(m, m2...) {
			if strings.HasSuffix(f, ".gz") {
				gz = append(gz, f)
			} else {
				plain = append(plain, f)
			}
		}
		return plain, gz
	}
	plain, _ := dataFiles()
	if len(plain) != 4 {
		t.Fatalf("%d data files before compression, want 4 (e4, 12 articles): %q", len(plain), plain)
	}
	old := time.Now().Add(-2 * time.Hour)
	for _, f := range plain {
		if err := os.Chtimes(f, old, old); err != nil {
			t.Fatal(err)
		}
	}

	// -n -v: the per-file lines, nothing compressed
	out := s.Run(t, "dexpireover", "-y", "-z", "-n", "-v")
	if n := strings.Count(out, " compression\n"); n != 2 || !strings.Contains(out, "2 files would be compressed") ||
		strings.Count(out, "not eligible for compression: proximity") != 2 {
		t.Errorf("dexpireover -y -z -n -v: want 2 \"compression\" and 2 \"proximity\" lines and \"2 files would be compressed\":\n%s", out)
	}
	if _, gz := dataFiles(); len(gz) != 0 {
		t.Fatalf("dexpireover -n compressed %q", gz)
	}

	// without -v: only the totals, no line per data file (the cron runs it so)
	out = s.Run(t, "dexpireover", "-y", "-z")
	plain, gz := dataFiles()
	if len(gz) != 2 || len(plain) != 2 {
		t.Fatalf("after dexpireover -z: %d .gz and %d plain data files, want 2 and 2:\n%q\n%q\n%s",
			len(gz), len(plain), gz, plain, out)
	}
	if !strings.Contains(out, "Compressed 2 files") {
		t.Errorf("dexpireover -y -z: no \"Compressed 2 files\":\n%s", out)
	}
	for _, l := range []string{"No .gz on ", "not eligible for compression", " compression\n"} {
		if strings.Contains(out, l) {
			t.Errorf("dexpireover -y -z without -v printed a per-file line (%q):\n%s", l, out)
		}
	}

	// a fresh dreaderd: no data file of before is still open in a cache
	s.Proc.Stop()
	_, port, _ := strings.Cut(s.Addr, ":")
	startReader(t, s, "[127.0.0.1]:"+port)

	after := read()
	for _, cmd := range slices.Sorted(maps.Keys(before)) {
		if !slices.Equal(after[cmd], before[cmd]) {
			t.Errorf("%s from gzipped data differs:\nbefore %q\nafter  %q", cmd, before[cmd], after[cmd])
		}
	}

	// a new article goes into the newest data file, not over a gzipped one
	feedReader(t, s, st, "e2e.test", article(13))
	c := s.Reader(t)
	testkit.WaitFor(t, 10*time.Second, "13 articles in GROUP e2e.test", func() bool {
		_, line := c.Cmd("GROUP e2e.test")
		return strings.HasPrefix(line, "211 13 1 13 ")
	})
	c.ExpectCmd(225, "HDR X-Gz-Note 1-13")
	if got := c.ReadDotLines(); len(got) != 13 || !slices.Equal(got[:12], before["HDR X-Gz-Note 1-12"]) {
		t.Errorf("HDR X-Gz-Note 1-13 after a new article: %q", got)
	}
	c.ExpectCmd(205, "QUIT")
	if _, gz := dataFiles(); len(gz) != 2 {
		t.Errorf("%d .gz data files after a new article, want the same 2: %q", len(gz), gz)
	}
}

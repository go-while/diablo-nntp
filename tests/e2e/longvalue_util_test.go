package e2e

import (
	testkit "github.com/go-while/nntp-testkit"
	"github.com/go-while/nntp-testkit/client"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/go-while/nntp-testkit/fakeserver"
)

// Tests for overlong command-line arguments and configuration values in the utilities: each
// program refuses the value, or works with it, instead of copying it into a fixed-size buffer
// past its end (found with an ASan build) or working on a name cut to fit.

// noSanitizerReport fails the test if out holds an AddressSanitizer or UBSan report (an ASan
// build of the old code reports the overflow there and exits).
func noSanitizerReport(tb testing.TB, out string) {
	tb.Helper()
	if strings.Contains(out, "AddressSanitizer") || strings.Contains(out, "runtime error:") {
		tb.Errorf("sanitizer report:\n%s", out)
	}
}

// TestDhisexpireLongFileName: dhisexpire built the names of the new history and of the
// backup with sprintf("%s.new") into PATH_MAX buffers, from the history file argument. A
// 5000-character argument ran past them; now it is refused before anything is opened.
func TestDhisexpireLongFileName(t *testing.T) {
	s := NewSite(t, SiteOptions{NoStart: true})
	long := s.Dir + "/" + strings.Repeat("h", 5000)
	for _, args := range [][]string{
		{long},                            // FileName.new and FileName.bak
		{s.Dir + "/dhistory", long},       // new-history argument
		{"-o", long, s.Dir + "/dhistory"}, // -o backup name
	} {
		out, err := s.TryRun("dhisexpire", args...)
		noSanitizerReport(t, out)
		if err == nil || !strings.Contains(out, "file name too long") {
			t.Errorf("dhisexpire with a 5000-character name (%d args): err %v, want a refusal; output:\n%.400s", len(args), err, out)
		}
	}
}

// TestDreadartLongDumpLine: "dreadart -f file" reads DUMP lines (didump's output) of up to 8191
// bytes and took the first field with an unbounded sscanf %s into a 250-byte buffer. A long
// field is now skipped like any other line that does not parse.
func TestDreadartLongDumpLine(t *testing.T) {
	s := NewSite(t, SiteOptions{})
	p := s.Peer(t)
	id := msgID(t)
	p.ExpectCmd(335, "IHAVE %s", id) // creates dhistory
	p.MustSend(client.Stuff(newArticle(id, "e2e.test", "history", "body")))
	p.ExpectStatus(235, "IHAVE")
	p.ExpectCmd(205, "QUIT")

	in := filepath.Join(s.Dir, "dump.txt")
	line := "DUMP " + strings.Repeat("a", 4000) + " gm=1 ex=1 boff=1 bsize=1\n"
	if err := os.WriteFile(in, []byte(line), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := s.TryRun("dreadart", "-f", in)
	noSanitizerReport(t, out)
	if err != nil {
		t.Errorf("dreadart -f with a long DUMP field: %v\n%.400s", err, out)
	}
}

// TestDspooloutLongLabel: dspoolout formatted the queue file names "label.S00000" into
// 256-byte buffers. A dnewsfeeds label of 300 characters ran past them, and ".label.seq" was
// cut to 255 characters, a different name, which dspoolout then created. The names now have
// room (PATH_MAX); such a label cannot name a queue file (a file name has at most 255
// characters), so nothing is done for it, and the other labels are processed as before.
func TestDspooloutLongLabel(t *testing.T) {
	long := strings.Repeat("l", 300)
	feeds := GlobalLabel + `label ` + long + `
    hostname 127.0.0.1
    addgroup *
end
label out
    hostname 127.0.0.1
    addgroup *
end
`
	s := NewSite(t, SiteOptions{Newsfeeds: feeds, NoStart: true})
	s.WriteFile(t, "dqueue/out", "D.00000000/B.0000 <a@b> 0,1\n")
	out, err := s.TryRun("dspoolout", "-n")
	noSanitizerReport(t, out)
	if err != nil {
		t.Fatalf("dspoolout: %v\n%.600s", err, out)
	}
	if !strings.Contains(out, "add out.S00000") {
		t.Errorf("the queue of the other label was not rotated:\n%.600s", out)
	}
	entries, _ := os.ReadDir(filepath.Join(s.Dir, "dqueue"))
	for _, e := range entries {
		if strings.Contains(e.Name(), "lll") {
			t.Errorf("dspoolout created %q for the long label", e.Name())
		}
	}
}

// TestDnewslinkDqueuePattern: dnewslink used path_dqueue as the format string of a snprintf()
// (with path_home as the argument), while diablo and the other programs expand it as a path
// pattern. A '%' in the directory name other than "%s" made dnewslink look for the queue in a
// different directory; now it expands the pattern like everyone else.
func TestDnewslinkDqueuePattern(t *testing.T) {
	peer, got := newPeer(t, &fakeserver.PeerOptions{})
	s := NewSite(t, SiteOptions{
		Newsfeeds: OutNewsfeeds,
		Config:    map[string]string{"path_dqueue": "@home@/dq%dq"},
		Setup: func(tb testing.TB, s *Site) {
			if err := os.Mkdir(filepath.Join(s.Dir, "dq%dq"), 0o755); err != nil {
				tb.Fatal(err)
			}
		},
	})
	ids := s.Inject(t, "e2e.test", 2)
	testkit.WaitFor(t, 10*time.Second, "the queue in dq%dq", func() bool {
		s.Dicmd(t, "flush")
		b, _ := os.ReadFile(filepath.Join(s.Dir, "dq%dq", OutLabel))
		return strings.Count(string(b), "\n") >= 2
	})
	out := s.dnewslink(t, peer, OutLabel)
	noSanitizerReport(t, out)
	if !slices.Equal(got.list(), ids) {
		t.Errorf("peer accepted %q, want %q\ndnewslink output:\n%s", got.list(), ids, out)
	}
}

// TestDrequeueLongCSize: drequeue reads SOUT lines on stdin and queues them like diablo's
// writeFeedOut(), but did not count the compressed-size field in its "does the line fit"
// test, so a long last field ran past the one-page queue buffer. The line is now too long and
// not queued; drequeue finishes normally and queues the next line.
func TestDrequeueLongCSize(t *testing.T) {
	s := NewSite(t, SiteOptions{NoStart: true})
	sout := func(id, csize string) string {
		return strings.Join([]string{"SOUT", "D.00000000/B.0000", "0,10", id, "e2e.test", "world",
			"e2e.other!not-for-mail", "0", "000000", csize}, "\t") + "\n"
	}
	in := sout("<long@e2e.test>", strings.Repeat("9", 7000)) + sout("<short@e2e.test>", "5")
	cmd := exec.Command(filepath.Join(s.Bin, "drequeue"), "peer")
	cmd.Dir = s.Dir
	cmd.Env = append(os.Environ(), s.Env...)
	cmd.Stdin = strings.NewReader(in)
	b, err := cmd.CombinedOutput()
	out := string(b)
	noSanitizerReport(t, out)
	if err != nil {
		t.Fatalf("drequeue: %v\n%.400s", err, out)
	}
	q := s.ReadFile(t, "dqueue/peer")
	if strings.Contains(q, "<long@e2e.test>") || !strings.Contains(q, "<short@e2e.test> 0,10 C5") {
		t.Errorf("dqueue/peer: %.300q", q)
	}
}

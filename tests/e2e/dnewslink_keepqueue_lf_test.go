package e2e

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/go-while/nntp-testkit/fakeserver"
)

// readline() copies at most 1023 bytes of a queue line of 1024 bytes or more (with its newline)
// and drops the rest, its newline included.
// A line kept in the queue after a spool error must still end in a newline, or the next
// kept line is glued onto it and lost at the next run.
func TestDnewslinkKeptLongLineKeepsNewline(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root opens a file of mode 000")
	}
	s := NewSite(t, SiteOptions{NoStart: true})
	dnlSpool(t, s)
	if err := os.Chmod(filepath.Join(s.Dir, "spool", "news", "D.00000001", "B.0001"), 0); err != nil {
		t.Fatal(err)
	}
	srv := fakeserver.New(t, fakeserver.Options{Greeting: "200 fake", Handler: fakeserver.LineHandler(func(_ *fakeserver.Conn, line string) ([]string, bool) {
		line = strings.ToLower(strings.TrimSpace(line))
		switch {
		case strings.HasPrefix(line, "mode stream"):
			return []string{"203 stream ok"}, false
		case strings.HasPrefix(line, "quit"):
			return []string{"205 bye"}, true
		}
		return nil, false
	})})
	// 1024 bytes with the newline: the longest line readline() takes whole minus one.
	first := "D.00000001/B.0001 <keeplf-1@e2e.test> 100,100"
	first += strings.Repeat(" ", 1023-len(first))
	second := "D.00000001/B.0001 <keeplf-2@e2e.test> 200,100"
	if err := os.WriteFile(filepath.Join(s.Dir, "dqueue", "out.S00000"), []byte(first+"\n"+second+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := s.TryRun("dnewslink", "-Aall", "-N", "1", "-S", "0", "-b", "out.S%05d", "-h", "127.0.0.1",
		"-P", fmt.Sprint(srv.Port()), "-t", "5", "-w", "1")
	t.Logf("dnewslink: err=%v\n%s", err, out)
	got := dnlQueue(t, s, "out.S00000")
	if n := strings.Count(got, "\n"); n != 2 {
		t.Errorf("kept queue has %d newlines, want 2 (one per entry); queue now: %q", n, got)
	}
	lines := strings.Split(strings.TrimSuffix(got, "\n"), "\n")
	if len(lines) != 2 || !strings.HasPrefix(lines[0], "D.00000001/B.0001 <keeplf-1@e2e.test> ") || lines[1] != second {
		t.Errorf("kept queue differs: got %q, want the first entry (cut at 1023 bytes) and %q on its own line", got, second)
	}
}

// The same for the copy of the unprocessed rest of the queue when dnewslink stops early: the
// peer defers the first two entries and goes away at the third, the fourth entry is 1024
// bytes with its newline and the fifth follows it.
func TestDnewslinkLongLineKeptInQueueRest(t *testing.T) {
	s := NewSite(t, SiteOptions{NoStart: true})
	dnlSpool(t, s)
	srv := fakeserver.New(t, fakeserver.Options{Greeting: "200 fake", Handler: fakeserver.LineHandler(func(_ *fakeserver.Conn, line string) ([]string, bool) {
		line = strings.ToLower(strings.TrimSpace(line))
		switch {
		case strings.HasPrefix(line, "mode stream"):
			return []string{"203 stream ok"}, false
		case strings.HasPrefix(line, "check <kl1@") || strings.HasPrefix(line, "check <kl2@"):
			return []string{"431 " + strings.Fields(line)[1] + " try later"}, false
		case strings.HasPrefix(line, "check <kl3@"):
			return []string{"400 going away"}, true
		}
		return nil, false
	})})
	var q strings.Builder
	for i := 1; i <= 5; i++ {
		l := fmt.Sprintf("D.00000001/B.0001 <kl%d@e2e.test> %d,100", i, (i-1)*100)
		if i == 4 {
			l += strings.Repeat(" ", 1023-len(l))
		}
		q.WriteString(l + "\n")
	}
	if err := os.WriteFile(filepath.Join(s.Dir, "dqueue", "out.S00000"), []byte(q.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := s.TryRun("dnewslink", "-N", "1", "-S", "0", "-b", "out.S%05d", "-h", "127.0.0.1",
		"-P", strconv.Itoa(srv.Port()), "-t", "5", "-M", "1")
	t.Logf("dnewslink: err=%v\n%s", err, out)
	got := dnlQueue(t, s, "out.S00000")
	for i := 1; i <= 5; i++ {
		id := fmt.Sprintf("<kl%d@e2e.test>", i)
		if n := strings.Count(got, id); n != 1 {
			t.Errorf("%s is in the queue %d times, want 1", id, n)
		}
	}
	lines := strings.Split(strings.TrimSuffix(got, "\n"), "\n")
	if len(lines) != 5 {
		t.Errorf("kept queue has %d lines, want 5 (one per entry); queue now: %q", len(lines), got)
	}
	for _, l := range lines {
		if strings.Count(l, "<kl") != 1 {
			t.Errorf("queue line holds more or less than one entry: %.80q", l)
		}
	}
}

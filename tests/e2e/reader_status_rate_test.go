package e2e

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
)

// writeCalls returns the sum of the write system calls made so far by the given
// processes (syscw in /proc/<pid>/io).
func writeCalls(t *testing.T, pids []int) int64 {
	t.Helper()
	var sum int64
	for _, pid := range pids {
		b, err := os.ReadFile(fmt.Sprintf("/proc/%d/io", pid))
		if err != nil {
			t.Skipf("cannot read /proc/%d/io: %v", pid, err)
		}
		for _, l := range strings.Split(string(b), "\n") {
			if v, ok := strings.CutPrefix(l, "syscw: "); ok {
				n, _ := strconv.ParseInt(strings.TrimSpace(v), 10, 64)
				sum += n
			}
		}
	}
	return sum
}

// TestReaderStatusFileWriteRate: dreaderd keeps the last command of every connection in
// dreaderd.status. It wrote that file twice per command (the command, then "(idle)"), so a
// client sending 2000 short commands cost 4000 extra writes. Now a connection's status goes
// to the file at most once a second, and the last state still gets there within about a
// second after the commands stop. A state that was held back never lands on the line of
// the closed connection or of the next one in the same slot, and the main process does not
// sleep on a held-back update of its counters.
func TestReaderStatusFileWriteRate(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("counts write calls through /proc/<pid>/io")
	}
	_, spool := spoolStore(t)
	// readerdetaillog off: without the two log lines per command the reply is the only
	// write a command needs, so the status writes stand out.
	s := NewReaderSite(t, ReaderOptions{Spool: spool,
		Config: map[string]string{"readerdetaillog": "off"}})
	c := s.Reader(t)
	c.ExpectCmd(111, "DATE")

	const n = 2000
	pids := sitePids(t, s, "dreaderd")
	before := writeCalls(t, pids)
	for i := 0; i < n; i++ {
		c.ExpectCmd(111, "DATE")
	}
	writes := writeCalls(t, pids) - before
	// One write per reply is needed; before the change there were two more per command.
	if writes >= n+n/2 {
		t.Errorf("%d commands made dreaderd do %d writes, want fewer than %d (status file written per command?)",
			n, writes, n+n/2)
	}
	t.Logf("%d commands, %d writes", n, writes)

	status := filepath.Join(s.Dir, "log", "dreaderd.status")
	readStatus := func() string {
		b, err := os.ReadFile(status)
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	// lineWith returns the status line that holds mark, "" when there is none.
	lineWith := func(mark string) string {
		for _, l := range strings.Split(readStatus(), "\n") {
			if strings.Contains(l, mark) {
				return l
			}
		}
		return ""
	}

	// A state that is held back must still reach the file. After a quiet second the
	// command goes to the file at once and the "(idle)" that follows it in the same second
	// is held back: the file shows the command first and "(idle)" about a second later.
	// (When the two fall into different seconds nothing is held back; try again.)
	held := false
	for try := 0; try < 8 && !held; try++ {
		time.Sleep(1200 * time.Millisecond)
		c.ExpectCmd(111, "DATE")
		if lineWith("DATE") == "" {
			continue
		}
		held = true
		for deadline := time.Now().Add(4 * time.Second); lineWith("DATE") != ""; time.Sleep(50 * time.Millisecond) {
			if time.Now().After(deadline) {
				t.Fatalf("4 s after the command dreaderd.status still shows it, not the idle connection:\n%s", readStatus())
			}
		}
		if lineWith("(idle)") == "" {
			t.Errorf("the connection is idle, dreaderd.status does not say so:\n%s", readStatus())
		}
	}
	if !held {
		t.Errorf("never saw a held-back status in 8 tries; is the status still written per command?")
	}

	// A held-back state must not land on the line of what came after it. Close with one
	// held back: the line shows the closed connection and keeps showing it.
	time.Sleep(1200 * time.Millisecond)
	c.ExpectCmd(111, "DATE")
	c.ExpectCmd(205, "QUIT")
	for deadline := time.Now().Add(4 * time.Second); lineWith("CLSD") == ""; time.Sleep(50 * time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatalf("no closed connection in dreaderd.status 4 s after QUIT:\n%s", readStatus())
		}
	}
	for i := 0; i < 25; i++ {
		if l := lineWith("CLSD"); !strings.Contains(l, "(closed)") {
			t.Fatalf("the line of the closed connection was overwritten: %q", l)
		}
		time.Sleep(100 * time.Millisecond)
	}

	// The same slot for a new connection: its line is its own from the start.
	time.Sleep(1200 * time.Millisecond)
	c1 := s.Reader(t)
	c1.ExpectCmd(111, "DATE")
	c1.ExpectCmd(205, "QUIT")
	c2 := s.Reader(t)
	for i := 0; i < 25; i++ {
		got := readStatus()
		if strings.Contains(got, "QUIT") || strings.Contains(got, "DATE") {
			t.Fatalf("a command of the connection before shows on a line of dreaderd.status:\n%s", got)
		}
		time.Sleep(100 * time.Millisecond)
	}
	if lineWith("ACTV") == "" {
		t.Errorf("the open connection is not in dreaderd.status:\n%s", readStatus())
	}

	// The main process has a line of counters and sleeps up to ten seconds: its held-back
	// update must not wait that long. One more connection shows within about a second.
	connects := func() int {
		l := lineWith("Connect=")
		i := strings.Index(l, "Connect=")
		if i < 0 {
			t.Fatalf("no line of the main process in dreaderd.status:\n%s", readStatus())
		}
		n, _ := strconv.Atoi(strings.Fields(l[i+len("Connect="):])[0])
		return n
	}
	time.Sleep(2500 * time.Millisecond)
	before = int64(connects())
	c3 := s.Reader(t)
	c4 := s.Reader(t)
	for deadline := time.Now().Add(4 * time.Second); int64(connects()) != before+2; time.Sleep(50 * time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatalf("two more connections, 4 s later dreaderd.status has Connect=%d, want %d", connects(), before+2)
		}
	}
	c4.ExpectCmd(205, "QUIT")
	c3.ExpectCmd(205, "QUIT")
	c2.ExpectCmd(205, "QUIT")
}

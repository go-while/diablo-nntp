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
// second after the commands stop.
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

	// The held-back status must still reach the file: the connection is idle now.
	status := filepath.Join(s.Dir, "log", "dreaderd.status")
	var got string
	for deadline := time.Now().Add(5 * time.Second); ; time.Sleep(100 * time.Millisecond) {
		b, err := os.ReadFile(status)
		if err != nil {
			t.Fatal(err)
		}
		got = string(b)
		if strings.Contains(got, "(idle)") && !strings.Contains(strings.ToLower(got), " date") {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("5 s after the last command dreaderd.status does not show the idle connection:\n%s", got)
		}
	}
	c.ExpectCmd(205, "QUIT")
}

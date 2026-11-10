package e2e

import (
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
)

// procNice returns the nice value of a process from /proc/<pid>/stat (Linux).
func procNice(pid int) (int, bool) {
	b, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return 0, false
	}
	// the fields after the command name, which is in parentheses and may hold blanks
	f := strings.Fields(string(b)[strings.LastIndexByte(string(b), ')')+1:])
	if len(f) < 17 {
		return 0, false
	}
	n, err := strconv.Atoi(f[16]) // field 19 of the line
	return n, err == nil
}

// TestDeepRevIncomingPriority (L2-12): the dnewsfeeds option "incomingpriority" is the nice
// value of the label's incoming sessions, 0..20 (samples/dnewsfeeds), but it was read with
// enabled(), the yes/no parser, which knows only a leading "y", "on" or "1": 10 and 15 gave
// 1, and 5 or 20 gave 0. The number must be taken as it is.
//
// And it must be used: the nice value was set right after the fork, before the label of the
// connection is known, so no session ever ran with it. The session process of the peer must
// have the label's value added to the nice value of the main process (checked on Linux).
func TestDeepRevIncomingPriority(t *testing.T) {
	for _, c := range []struct{ value, want string }{
		{"10", "10"},
		{"5", "5"},
		{"20", "20"},
		{"0", "0"},
	} {
		t.Run(c.value, func(t *testing.T) {
			s := NewSite(t, SiteOptions{Newsfeeds: GlobalLabel + "label peer\n    inhost " + PeerIP +
				"\n    addgroup *\n    incomingpriority " + c.value + "\nend\n"})
			p := s.Peer(t)
			if want, _ := strconv.Atoi(c.want); want > 0 && runtime.GOOS == "linux" {
				base, ok := procNice(s.Proc.Pid())
				if !ok {
					t.Fatalf("cannot read the nice value of the main process")
				}
				found := false
				var seen []int
				for deadline := time.Now().Add(3 * time.Second); !found && time.Now().Before(deadline); time.Sleep(50 * time.Millisecond) {
					seen = seen[:0]
					for _, pid := range procChildren(t, s.Proc.Pid()) {
						if n, ok := procNice(pid); ok {
							seen = append(seen, n)
							found = found || n == min(base+want, 19)
						}
					}
				}
				if !found {
					t.Errorf("incomingpriority %s: no session process with nice %d (main process %d); children have %v",
						c.value, min(base+want, 19), base, seen)
				}
			}
			p.ExpectCmd(205, "QUIT")
			out := s.Dicmd(t, "dumpfeed", "peer")
			if want := "IncomingPriority: " + c.want + "\n"; !strings.Contains(out, want) {
				t.Errorf("incomingpriority %s: dicmd dumpfeed peer has no %q:\n%s", c.value, want, out)
			}
		})
	}
}

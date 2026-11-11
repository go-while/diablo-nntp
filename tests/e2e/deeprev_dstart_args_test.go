package e2e

import (
	"strconv"
	"strings"
	"testing"

	testkit "github.com/go-while/nntp-testkit"
)

// TestDeepRevDstartManyArgs (L8-12): dstart builds the argument vector of the daemon it
// starts in "char *argv[255]" and copied its own arguments into it without a bound: from 254
// arguments on it wrote past the array on its stack (dstart is meant to be installed setuid
// root).
//
// With more arguments than the array holds dstart must say so and exit before it does
// anything else; the largest number that fits is still taken.
func TestDeepRevDstartManyArgs(t *testing.T) {
	s := NewSite(t, SiteOptions{NoStart: true})
	args := func(n int) []string {
		a := []string{"diablo", "-P127.0.0.1:" + strconv.Itoa(testkit.FreePort(t))}
		for len(a) < n {
			a = append(a, "-x")
		}
		return a
	}

	// 253 arguments fit: dstart goes on to bind and to exec @home@/dbin/diablo, which the
	// test site does not have.
	out, err := s.TryRun("dstart", args(253)...)
	if strings.Contains(out, "too many arguments") {
		t.Errorf("dstart with 253 arguments: err=%v, refused:\n%s", err, out)
	}

	for _, n := range []int{254, 300, 2000} {
		out, err := s.TryRun("dstart", args(n)...)
		if err == nil || !strings.Contains(out, "too many arguments") || strings.Contains(out, "Listening on") {
			t.Errorf("dstart with %d arguments: err=%v, want \"too many arguments\" and nothing else:\n%s", n, err, out)
		}
	}
}

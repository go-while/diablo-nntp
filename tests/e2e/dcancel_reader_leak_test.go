package e2e

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestDcancelReaderManyLines (D-286): dcancel -r opened the reader active file (a
// descriptor and a mapping) for every input line, and for a group in active also the group's
// over.* file (a descriptor and a mapping), and never closed or unmapped them, so a run stopped
// with "Unable to open active file" once the descriptors ran out (about 500 lines of existing
// groups with a limit of 1024). The active file is now opened once, and the overview index is
// unmapped and closed after each line.
//
// The test runs dcancel -r with a limit of 64 descriptors on 302 lines: the article to cancel
// first, then lines for an article that is not there and for a group that is not there, and
// last an article of another group. Every line is answered, both articles are cancelled and
// dcancel exits 0. Fails on main: dcancel stops with exit status 1 after about 40 lines.
func TestDcancelReaderManyLines(t *testing.T) {
	s, _, _ := overReaderSite(t)
	var in strings.Builder
	in.WriteString("e2e.test:1\n")
	const noise = 300
	for i := 0; i < noise; i++ {
		if i%2 == 0 {
			fmt.Fprintf(&in, "e2e.test:%d\n", 1000+i) // no such article
		} else {
			in.WriteString("nogrp.nosuchgroup:1\n") // no such group
		}
	}
	in.WriteString("e2e.other:1\n")

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "/bin/sh", "-c", `ulimit -n 64 && exec "$0" -r`, filepath.Join(s.Bin, "dcancel"))
	cmd.Dir = s.Dir
	cmd.Env = append(os.Environ(), s.Env...)
	cmd.Stdin = strings.NewReader(in.String())
	b, err := cmd.CombinedOutput()
	out := string(b)
	short := out
	if len(short) > 3000 {
		short = short[:1500] + "\n...\n" + short[len(short)-1500:]
	}
	if err != nil {
		t.Errorf("dcancel -r with 64 descriptors on %d lines: %v\n%s", noise+2, err, short)
	}
	for _, want := range []string{"1 in e2e.test cancelled", "1 in e2e.other cancelled"} {
		if !strings.Contains(out, want) {
			t.Errorf("no %q in the output of dcancel -r:\n%s", want, short)
		}
	}
	if n := strings.Count(out, "Unable to cancel "); n != noise {
		t.Errorf("%d lines answered \"Unable to cancel\", want %d:\n%s", n, noise, short)
	}
	if strings.Contains(out, "Unable to open active file") {
		t.Errorf("dcancel ran out of descriptors:\n%s", short)
	}
}

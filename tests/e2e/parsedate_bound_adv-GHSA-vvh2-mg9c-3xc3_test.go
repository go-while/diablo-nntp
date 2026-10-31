package e2e

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestParsedateNoReadPastBuffer: parsedate() (lib/parsedate.c) must not read past the end of
// the buffer it is given. When a Date value ends right after the year, the old code did
//
//	ti.tm_hour = strtol(d, &d, 10);	// d is on the terminating NUL, converts nothing
//	d++;				// steps PAST the NUL
//	ti.tm_min  = strtol(d, &d, 10);	// reads the byte after the string
//
// The feeder keeps the Date header in char dateBuf[64] (util/diablo.c LoadArticle); a header
// whose stored form ends at the year puts the NUL in dateBuf[63], so the stray read is at
// dateBuf[64] - one past the array, into the next stack local (nntpPostingHost). Measured with
// AddressSanitizer on Leap/glibc and Alpine/musl as a stack-buffer-overflow READ.
//
// This test drives the ACTUAL lib/parsedate.c: it lifts the function bodies out of the file as
// they stand (everything after the project include, which the parser does not otherwise use),
// compiles them with a driver that places dateBuf immediately before a known sentinel "47", and
// checks the minute parsedate returns. If the read runs past dateBuf it picks up the sentinel
// and the minute is 47; bounded correctly, the minute is 0. Fails on c30c230, passes with the
// one-line guard.
func TestParsedateNoReadPastBuffer(t *testing.T) {
	cc := lookCC(t)

	src, err := os.ReadFile(filepath.Join(repoRoot(t), "lib", "parsedate.c"))
	if err != nil {
		t.Fatal(err)
	}
	// Everything after `#include "defs.h"` is the parser: parsedate() and parsemonth(), which
	// use only libc. Drop the project include and the Prototype line, supply libc headers.
	body := string(src)
	if i := strings.Index(body, "#include \"defs.h\""); i >= 0 {
		body = body[i+len("#include \"defs.h\""):]
	} else {
		t.Fatalf("lib/parsedate.c: no #include \"defs.h\" to cut at")
	}

	prog := `#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <strings.h>
#include <ctype.h>
#include <time.h>
#define Prototype
` + body + `

/* dateBuf is immediately followed by a known sentinel, so a read past dateBuf[63] is
 * deterministic: it picks up '4','7'. */
struct frame { char dateBuf[64]; char after[8]; };

int main(void) {
    struct frame f;
    memset(&f, 0, sizeof(f));
    /* "(49 a's) 28 Oct 2000": 63 chars, year's last digit at dateBuf[62], NUL at dateBuf[63] */
    const char *v = "(aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa) 28 Oct 2000";
    strncpy(f.dateBuf, v, sizeof(f.dateBuf) - 1);
    f.dateBuf[sizeof(f.dateBuf) - 1] = 0;
    f.after[0] = '4'; f.after[1] = '7'; f.after[2] = 0;
    setenv("TZ", "UTC", 1); tzset();
    time_t tt = parsedate(f.dateBuf);
    if (tt == (time_t)-1) { printf("MIN=-1\n"); return 0; }
    struct tm g = *gmtime(&tt);
    printf("MIN=%d\n", g.tm_min);
    return 0;
}
`
	dir := t.TempDir()
	csrc := filepath.Join(dir, "pd.c")
	if err := os.WriteFile(csrc, []byte(prog), 0644); err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(dir, "pd")
	out, err := exec.Command(cc, "-O2", "-w", "-o", bin, csrc).CombinedOutput()
	if err != nil {
		t.Fatalf("compiling the extracted parsedate: %v\n%s", err, out)
	}
	got, err := exec.Command(bin).CombinedOutput()
	if err != nil {
		t.Fatalf("running the parsedate driver: %v\n%s", err, got)
	}
	line := strings.TrimSpace(string(got))
	min, ok := strings.CutPrefix(line, "MIN=")
	if !ok {
		t.Fatalf("driver output %q, want MIN=<n>", line)
	}
	if min != "0" {
		t.Errorf("parsedate read past dateBuf: minute = %s, want 0 (it picked up the sentinel after the buffer)", min)
	}
}

func lookCC(t *testing.T) string {
	t.Helper()
	for _, c := range []string{os.Getenv("CC"), "cc", "gcc", "clang"} {
		if c == "" {
			continue
		}
		if p, err := exec.LookPath(c); err == nil {
			return p
		}
	}
	t.Skip("no C compiler (cc/gcc/clang) on PATH")
	return ""
}

// repoRoot returns the source tree that holds lib/parsedate.c. The e2e binary is run from a
// directory away from its source (the CI ships the binary and the tree separately), so the
// source is found first through DIABLO_BIN, whose dbin directory is a sibling of lib in the
// build tree; an in-tree `go test` has no DIABLO_BIN and falls back to walking up from the CWD.
func repoRoot(t *testing.T) string {
	t.Helper()
	if d := os.Getenv("E2E_SRC"); d != "" {
		if _, err := os.Stat(filepath.Join(d, "lib", "parsedate.c")); err == nil {
			return d
		}
	}
	if b := os.Getenv(EnvBin); b != "" {
		r := filepath.Dir(b) // dbin -> tree root
		if _, err := os.Stat(filepath.Join(r, "lib", "parsedate.c")); err == nil {
			return r
		}
	}
	d, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(d, "lib", "parsedate.c")); err == nil {
			return d
		}
		p := filepath.Dir(d)
		if p == d {
			t.Skip("lib/parsedate.c not found via DIABLO_BIN or above the working directory")
		}
		d = p
	}
}

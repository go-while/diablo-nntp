package e2e

// zallocStrTrim2() in lib/zalloc.c sizes its result in a counting pass and
// fills it in a second pass.  The "skip whitespace after the separator" flag
// was initialised once, outside the two passes, so the second pass started in
// whatever state the first pass ended in.  For a folded header whose value
// starts on the continuation line ("Newsgroups:\r\n\tfoo.bar") that made the
// passes disagree when the value ended in the separator (one byte allocated
// and not written), and kept the leading tab in the result when it did not
// ("\tfoo.bar" as a group name).  The fix starts both passes in the
// after-separator state.
//
// The function is lifted out of lib/zalloc.c with a stub zalloc() that
// records the allocation size, so the driver can check both the trimmed text
// and that the two passes agree (strlen(result) + 1 == bytes allocated).
//
// Needs a C compiler and the source tree; skips otherwise, and fails instead
// when E2E_REQUIRE_SRC=1 is set (see wmSkipf in wildmat_unit_test.go).

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestZallocStrTrim2Passes(t *testing.T) {
	cc := wmCC(t)
	root := wildmatSource(t) // the tree root; lib/zalloc.c sits beside lib/wildmat.c
	b, err := os.ReadFile(filepath.Join(root, "lib", "zalloc.c"))
	if err != nil {
		t.Fatal(err)
	}

	// Keep zallocStrTrim2 only: from its definition to the closing brace.
	src := string(b)
	i := strings.Index(src, "char *\nzallocStrTrim2(")
	if i < 0 {
		t.Fatalf("lib/zalloc.c: no zallocStrTrim2 definition to cut at")
	}
	j := strings.Index(src[i:], "\n}\n")
	if j < 0 {
		t.Fatalf("lib/zalloc.c: no end to zallocStrTrim2")
	}
	fn := src[i : i+j+3]

	prog := zallocTrimDriverHead + fn + zallocTrimDriverTail
	dir := t.TempDir()
	csrc := filepath.Join(dir, "zt.c")
	if err := os.WriteFile(csrc, []byte(prog), 0o644); err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(dir, "zt")
	if out, err := exec.Command(cc, "-O1", "-g", "-std=gnu99", "-o", bin, csrc).CombinedOutput(); err != nil {
		t.Fatalf("compiling the zallocStrTrim2 driver: %v\n%s", err, out)
	}
	got, err := exec.Command(bin).CombinedOutput()
	if err != nil {
		t.Fatalf("zallocStrTrim2 driver failed: %v\n%s", err, got)
	}
	if !strings.Contains(string(got), "cases=11 fails=0") {
		t.Fatalf("zallocStrTrim2 driver output %q, want cases=11 fails=0", got)
	}
}

const zallocTrimDriverHead = `#include <stdio.h>
#include <stdlib.h>
#include <string.h>

typedef struct MemPool MemPool;
static int lastAlloc;

static void *
zalloc(MemPool **pmp, int n)
{
    (void)pmp;
    lastAlloc = n;
    return malloc(n);
}

`

const zallocTrimDriverTail = `
static const struct {
    int sep;
    const char *in;
    const char *want;
} cases[] = {
    /* the example in the function's comment */
    { ',', "foo.bar1, foo.bar2,\r\n\tfoo.bar3", "foo.bar1,foo.bar2,foo.bar3" },
    { ',', "  foo, bar  \r\n", "foo,bar" },
    { ',', "a,\t b", "a,b" },
    { ',', "", "" },
    /* folded header, value starts on the continuation line */
    { ',', "\r\n\tfoo.bar", "foo.bar" },
    { ',', "\r\n\tfoo.bar,", "foo.bar," },
    { ',', " \r\n\tfoo.bar, baz", "foo.bar,baz" },
    { '!', " \r\n news.example!not-for-mail", "news.example!not-for-mail" },
    /* no separator: only the ends and the line breaks are trimmed */
    { 0, " Hello  World \r\n", "Hello  World" },
    { 0, "\r\n\tHello", "Hello" },
    { 0, "a, b", "a, b" },
};

int
main(void)
{
    int k, fails = 0;
    int n = (int)(sizeof(cases) / sizeof(cases[0]));

    for (k = 0; k < n; ++k) {
	char *r = zallocStrTrim2(NULL, cases[k].sep, cases[k].in,
	    (int)strlen(cases[k].in));

	if (strcmp(r, cases[k].want) != 0) {
	    printf("case %d: got \"%s\", want \"%s\"\n", k, r, cases[k].want);
	    ++fails;
	} else if ((int)strlen(r) + 1 != lastAlloc) {
	    printf("case %d: passes disagree: %d bytes allocated, %d used\n",
		k, lastAlloc, (int)strlen(r) + 1);
	    ++fails;
	}
	free(r);
    }
    printf("cases=%d fails=%d\n", n, fails);
    return fails != 0;
}
`

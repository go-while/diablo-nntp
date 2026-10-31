package e2e

// GHSA wildmat XPAT: DoMatch() in lib/wildmat.c read past the end of an
// unterminated '[' character class.  There were two sites: the class loop
// stopped on the NUL and the arm's continue made the outer for-increment read
// one byte past it (site 1), and a trailing '-' took the range branch, whose
// *++p stepped onto the NUL and let the loop walk until a 0 or ']' byte
// (site 2).  The fix adds `p[1] != '\0'` to the range test and returns FALSE
// when the class loop stops on the NUL.
//
// This is the unit-level detector: the real matcher is lifted out of
// lib/wildmat.c and run under AddressSanitizer against the trigger set, with
// the pattern in a strdup(strlen+1) buffer (the same size zallocStr uses for
// the XPAT pattern).  It also pins the 70 documented match -> no-match flips
// for malformed classes ending in '-'; those make the test fail on the
// unpatched code even without ASAN.  The e2e XPAT test is
// wildmat_xpat_test.go.
//
// Needs a C compiler with AddressSanitizer and the source tree; skips
// otherwise, and fails instead when E2E_REQUIRE_SRC=1 is set.

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// wmSkipf skips, or fails when E2E_REQUIRE_SRC=1: a CI that is meant to have
// the source tree and a compiler must not pass vacuously.
func wmSkipf(t *testing.T, format string, args ...any) {
	t.Helper()
	if os.Getenv("E2E_REQUIRE_SRC") == "1" {
		t.Fatalf("E2E_REQUIRE_SRC=1 but a prerequisite is missing: "+format, args...)
	}
	t.Skipf(format, args...)
}

// wildmatSource finds the tree that holds lib/wildmat.c (E2E_SRC,
// DIABLO_BIN's parent, then walking up from the working directory).
func wildmatSource(t *testing.T) string {
	t.Helper()
	rel := filepath.Join("lib", "wildmat.c")
	if d := os.Getenv("E2E_SRC"); d != "" {
		if _, err := os.Stat(filepath.Join(d, rel)); err == nil {
			return d
		}
	}
	if b := os.Getenv(EnvBin); b != "" {
		r := filepath.Dir(b)
		if _, err := os.Stat(filepath.Join(r, rel)); err == nil {
			return r
		}
	}
	d, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(d, rel)); err == nil {
			return d
		}
		p := filepath.Dir(d)
		if p == d {
			wmSkipf(t, "%s not found via E2E_SRC, DIABLO_BIN or above the working directory", rel)
		}
		d = p
	}
}

// wmCC finds a C compiler, or skips the driver.
func wmCC(t *testing.T) string {
	t.Helper()
	for _, c := range []string{os.Getenv("CC"), "cc", "gcc", "clang"} {
		if c == "" {
			continue
		}
		if p, err := exec.LookPath(c); err == nil {
			return p
		}
	}
	wmSkipf(t, "no C compiler (cc/gcc/clang) on PATH")
	return ""
}

func TestWildmatUnterminatedClass(t *testing.T) {
	cc := wmCC(t)
	root := wildmatSource(t)
	b, err := os.ReadFile(filepath.Join(root, "lib", "wildmat.c"))
	if err != nil {
		t.Fatal(err)
	}

	// Keep the matcher only: drop the project include and the TEST main.
	src := string(b)
	inc := "#include\t\"defs.h\""
	i := strings.Index(src, inc)
	if i < 0 {
		inc = "#include \"defs.h\""
		i = strings.Index(src, inc)
	}
	if i < 0 {
		t.Fatalf("lib/wildmat.c: no #include \"defs.h\" to cut at")
	}
	body := src[i+len(inc):]
	for _, marker := range []string{"#if\tdefined(TEST)", "#if defined(TEST)"} {
		if j := strings.Index(body, marker); j >= 0 {
			body = body[:j]
			break
		}
	}
	if !strings.Contains(body, "DoMatch") {
		t.Fatalf("lib/wildmat.c: the extracted matcher has no DoMatch")
	}

	prog := "#include <stdio.h>\n#include <stdlib.h>\n#include <string.h>\n" +
		"#define Prototype\n" + body + wildmatDriverTail
	dir := t.TempDir()
	csrc := filepath.Join(dir, "wm.c")
	if err := os.WriteFile(csrc, []byte(prog), 0o644); err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(dir, "wm")
	out, err := exec.Command(cc, "-O1", "-g", "-fsanitize=address",
		"-fno-omit-frame-pointer", "-std=gnu99", "-o", bin, csrc).CombinedOutput()
	if err != nil {
		wmSkipf(t, "no AddressSanitizer support: %v\n%s", err, out)
	}
	got, err := exec.Command(bin).CombinedOutput()
	if err != nil {
		t.Fatalf("wildmat driver failed (over-read or wrong result): %v\n%s", err, got)
	}
	if !strings.Contains(string(got), "cases=84 fails=0") {
		t.Fatalf("wildmat driver output %q, want cases=84 fails=0", got)
	}
}

const wildmatDriverTail = `
/* The 70 pairs that change from "matched" to "no match" after the fix: all
 * are unterminated classes ending in '-', whose old result depended on the
 * byte read past the NUL (see changes/wildmat-xpat.md). */
struct wm_case { const char *pat; const char *text; int want; };
static const struct wm_case cases[] = {
    { "[]-", "]", 0 },
    { "[--", "-", 0 },
    { "[a-", "a", 0 },
    { "[[a-", "a", 0 },
    { "[][-", "]", 0 },
    { "[]^-", "]", 0 },
    { "[]!-", "]", 0 },
    { "[]a-", "a", 0 },
    { "[]a-", "]", 0 },
    { "[]*-", "]", 0 },
    { "[]?-", "]", 0 },
    { "[]\\-", "]", 0 },
    { "[] -", "]", 0 },
    { "[-[-", "-", 0 },
    { "[-^-", "-", 0 },
    { "[-!-", "-", 0 },
    { "[-a-", "a", 0 },
    { "[-a-", "-", 0 },
    { "[-*-", "-", 0 },
    { "[-?-", "-", 0 },
    { "[-\\-", "-", 0 },
    { "[- -", "-", 0 },
    { "[^[-", "a", 0 },
    { "[^[-", "]", 0 },
    { "[^[-", "-", 0 },
    { "[^]-", "a", 0 },
    { "[^]-", "-", 0 },
    { "[^--", "a", 0 },
    { "[^--", "]", 0 },
    { "[^^-", "a", 0 },
    { "[^^-", "]", 0 },
    { "[^^-", "-", 0 },
    { "[^!-", "a", 0 },
    { "[^!-", "]", 0 },
    { "[^!-", "-", 0 },
    { "[^a-", "]", 0 },
    { "[^a-", "-", 0 },
    { "[^*-", "a", 0 },
    { "[^*-", "]", 0 },
    { "[^*-", "-", 0 },
    { "[^?-", "a", 0 },
    { "[^?-", "]", 0 },
    { "[^?-", "-", 0 },
    { "[^\\-", "a", 0 },
    { "[^\\-", "]", 0 },
    { "[^\\-", "-", 0 },
    { "[^ -", "a", 0 },
    { "[^ -", "]", 0 },
    { "[^ -", "-", 0 },
    { "[!a-", "a", 0 },
    { "[a[-", "a", 0 },
    { "[a^-", "a", 0 },
    { "[a!-", "a", 0 },
    { "[aa-", "a", 0 },
    { "[a*-", "a", 0 },
    { "[a?-", "a", 0 },
    { "[a\\-", "a", 0 },
    { "[a -", "a", 0 },
    { "[*a-", "a", 0 },
    { "[?a-", "a", 0 },
    { "[\\a-", "a", 0 },
    { "[ a-", "a", 0 },
    { "][[-", "][", 0 },
    { "*[[-", "][", 0 },
    { "*[]-", "]", 0 },
    { "*[--", "-", 0 },
    { "*[--", "abc-", 0 },
    { "*[a-", "a", 0 },
    { "*[ -", "[]-^!a*?\\ ", 0 },
    { "?[[-", "][", 0 },

    /* well-formed controls: unchanged by the fix */
    { "[]",    "]", 0 },   /* unterminated: the first ] is a literal member */
    { "[]]",   "]", 1 },
    { "[a]",   "a", 1 },
    { "[a]",   "b", 0 },
    { "[-a]",  "-", 1 },
    { "[-a]",  "a", 1 },
    { "[a-z]", "m", 1 },
    { "[a-z]", "M", 0 },
    { "[^a]",  "b", 1 },
    { "[^a]",  "a", 0 },
    { "[!a]",  "!", 1 },
    { "[!a]",  "b", 0 },
    { "[",     "a", 0 },
    { "[a]",   "",  0 },
};

/* Every representative malformed pattern against several texts: with the
 * unpatched matcher this sweep trips ASAN at wildmat.c:94, :129 (or :117). */
static const char *sweep_pat[] = {
    "[", "[a", "[a-z", "[^", "[!a", "[a-", "[a-z-", "[^-", "[]-",
    "[]", "[a]", "[-a]", "[^a]", "[a-z]", "[!a]",
};
static const char *sweep_txt[] = { "", "a", "z", "-", "]", "A", "abc", "m" };

int
main(void)
{
    size_t i, j, fails = 0;
    const size_t nc = sizeof(cases) / sizeof(cases[0]);

    for (i = 0; i < nc; i++) {
	char *pat = strdup(cases[i].pat);
	char *txt = strdup(cases[i].text);
	int got = wildmat(txt, pat);

	if (got != cases[i].want) {
	    printf("FAIL pat=<%s> text=<%s> got=%d want=%d\n",
		   cases[i].pat, cases[i].text, got, cases[i].want);
	    fails++;
	}
	free(pat);
	free(txt);
    }

    for (i = 0; i < sizeof(sweep_pat) / sizeof(sweep_pat[0]); i++) {
	for (j = 0; j < sizeof(sweep_txt) / sizeof(sweep_txt[0]); j++) {
	    char *pat = strdup(sweep_pat[i]);
	    char *txt = strdup(sweep_txt[j]);

	    (void)wildmat(txt, pat);
	    free(pat);
	    free(txt);
	}
    }

    printf("cases=%zu fails=%zu\n", nc, fails);
    return(fails ? 1 : 0);
}
`

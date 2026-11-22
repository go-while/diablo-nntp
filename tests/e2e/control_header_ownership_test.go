package e2e

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// This extracted ExecuteControl scan checks replacement ownership with a
// synthetic allocator. The driver frees the last copies itself; it does not
// check the function's final cleanup or measure pool usage. Use -count=1
// because go test can reuse a cached pass when only control.c (outside the
// Go module) changes.
func TestControlHeaderOwnership(t *testing.T) {
	cc := wmCC(t)
	b, err := os.ReadFile(filepath.Join(wildmatSource(t), "dreaderd", "control.c"))
	if err != nil {
		t.Fatal(err)
	}
	_, scan, ok := strings.Cut(string(b), "void\nExecuteControl(")
	if !ok {
		t.Fatal("ExecuteControl definition not found")
	}
	const start = "const char *line;"
	_, scan, ok = strings.Cut(scan, start)
	scan, _, end := strings.Cut(scan, "body = line;")
	if !ok || !end {
		t.Fatal("ExecuteControl header scan not found")
	}
	dir := t.TempDir()
	src, bin := filepath.Join(dir, "control.c"), filepath.Join(dir, "control")
	if err := os.WriteFile(src, []byte(controlOwnershipDriver+start+scan+`
    if (nFrom != wantFrom || nNewsgroups != wantGroups)
        return 2;
    zfreeStr(&conn->co_MemPool, &from);
    zfreeStr(&conn->co_MemPool, &groups);
    zfreeStr(&conn->co_MemPool, &sender);
    return conn->co_MemPool != 0;
}
`), 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	run := func(name string, args ...string) ([]byte, error) {
		cmd := exec.CommandContext(ctx, name, args...)
		cmd.WaitDelay = time.Second // Bound the wait for inherited output pipes.
		return cmd.CombinedOutput()
	}
	if out, err := run(cc, "-o", bin, src); err != nil {
		t.Fatalf("compile header scan: %v\n%s", err, out)
	}
	for _, field := range []string{"From", "Newsgroups"} {
		t.Run(field, func(t *testing.T) {
			out, err := run(bin, field)
			if e, ok := err.(*exec.ExitError); ok && e.ExitCode() == 1 {
				t.Fatalf("%s copy left allocated after header cleanup", field)
			}
			if err != nil {
				t.Fatalf("run header scan: %v\n%s", err, out)
			}
		})
	}
}

const controlOwnershipDriver = `
#include <stdlib.h>
#include <string.h>
#include <strings.h>

static char *zallocStrTrim2(int *live, int sep, const char *s, int len)
{
    char *copy = malloc(len + 1);
    if (!copy)
        abort();
    memcpy(copy, s, len);
    copy[len] = 0;
    ++*live;
    return copy;
}

static void zfreeStr(int *live, char **s)
{
    if (*s) {
        free(*s);
        *s = NULL;
        --*live;
    }
}

int main(int argc, char **argv)
{
    struct { int co_MemPool; } c = {0}, *conn = &c;
    char *from = NULL, *sender = NULL, *groups = NULL;
    int wantFrom = strcmp(argv[1], "From") == 0 ? 2 : 0;
    int wantGroups = wantFrom ? 0 : 2;
    const char *art = wantFrom ? "From: a\nFrom: b\n\n" :
                                 "Newsgroups: a\nNewsgroups: b\n\n";
    int artLen = strlen(art);
`

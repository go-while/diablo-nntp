#!/bin/sh
# Run after ./configure, with GNU make (MAKE=gmake on BSD/illumos).
# Check a missing PATH_MAX, preservation of a system value, and bounded
# MakeGroupDirectory() input without requiring Go on GNU/Hurd.
set -eu
cd "$(dirname "$0")/../.."
tmp=$(mktemp -d "${TMPDIR:-/tmp}/diablo-d18.XXXXXXXX")
trap 'rm -rf "$tmp"' EXIT
trap 'exit 1' HUP INT TERM

cat > "$tmp/fallback.c" <<'C'
#include <limits.h>
#undef PATH_MAX
#include "compat/pathmax.h"
typedef char fallback_is_4096[(PATH_MAX == 4096) ? 1 : -1];
C

cat > "$tmp/check.c" <<'C'
#include <limits.h>
#ifdef PATH_MAX
enum { system_path_max = PATH_MAX };
#define CHECK_SYSTEM_PATH_MAX
#endif
#include "lib/defs.h"
typedef char spool_uses_path_max[(sizeof(((SpoolObject *)0)->so_Path) == PATH_MAX) ? 1 : -1];
#ifdef CHECK_SYSTEM_PATH_MAX
typedef char system_value_preserved[(PATH_MAX == system_path_max) ? 1 : -1];
#endif
#include <assert.h>

int main(int argc, char **argv)
{
    char too_long[PATH_MAX + 1];
    char path[PATH_MAX];
    int n;

    assert(argc == 2);
    memset(too_long, 'x', sizeof(too_long) - 1);
    too_long[sizeof(too_long) - 1] = 0;
    errno = 0;
    assert(MakeGroupDirectory(too_long) == -1);
    assert(errno == ENAMETOOLONG);

    n = snprintf(path, sizeof(path), "%s/a/b/file", argv[1]);
    assert(n > 0 && (size_t)n < sizeof(path));
    assert(MakeGroupDirectory(path) == 0);
    assert(MakeGroupDirectory(path) == 0);
    n = snprintf(path, sizeof(path), "%s/a/b", argv[1]);
    assert(n > 0 && (size_t)n < sizeof(path));
    assert(rmdir(path) == 0);
    n = snprintf(path, sizeof(path), "%s/a", argv[1]);
    assert(n > 0 && (size_t)n < sizeof(path));
    assert(rmdir(path) == 0);
    puts("D-18 portability checks passed");
    return 0;
}
C

# Reuse the configured compiler, optional-feature flags and link libraries.
cat > "$tmp/check.mk" <<'MAKE'
include Makefile
.PHONY: d18-portability-check
d18-portability-check: obj/libdiablo.a obj/libfilter.a
	$(CC) $(CPPFLAGS) $(CFLAGS) -c "$(D18_TMP)/fallback.c" -o "$(D18_TMP)/fallback.o"
	$(CC) $(CPPFLAGS) $(CFLAGS) "$(D18_TMP)/check.c" -o "$(D18_TMP)/check" $(LDFLAGS) $(LDLIBS)
	"$(D18_TMP)/check" "$(D18_TMP)"
MAKE
"${MAKE:-make}" -f "$tmp/check.mk" D18_TMP="$tmp" d18-portability-check

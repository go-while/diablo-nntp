#!/bin/sh
# Check that ./configure turns canlock on only for libcanlock 3.3 or later.
# Uses stand-in headers and libraries in a temporary directory, so it needs no
# libcanlock on the host, changes nothing in the tree (configure -n), and runs
# from the source directory or any other.  CC in the environment names the compiler.
set -eu
cd "$(dirname "$0")/../.."
tmp=$(mktemp -d "${TMPDIR:-/tmp}/diablo-canlock-probe.XXXXXXXX")
trap 'rm -rf "$tmp"' EXIT
trap 'exit 1' HUP INT TERM
cc=${CC:-cc}
fail=0

# mkstub DIR MAJOR MINOR: DIR/inc/libcanlock-3/canlock.h and DIR/lib/libcanlock.a
mkstub() {
    mkdir -p "$1/inc/libcanlock-3" "$1/lib"
    printf '#define CL_API_MAJOR %s\n#define CL_API_MINOR %s\nint cl_get_key(void);\n' "$2" "$3" > "$1/inc/libcanlock-3/canlock.h"
    printf 'int cl_get_key(void) { return 0; }\n' > "$1/stub.c"
    $cc -c -o "$1/stub.o" "$1/stub.c"
    ar rc "$1/lib/libcanlock.a" "$1/stub.o"
}

# check NAME VERSION EXPECTED-TEXT [configure option]
check() {
    mkstub "$tmp/$1" "${2%.*}" "${2#*.}"
    rc=0
    out=$(CC="$cc -I$tmp/$1/inc -L$tmp/$1/lib" ./configure -n ${4:-} 2>&1) || rc=$?
    line=$(printf '%s\n' "$out" | grep -i canlock | head -1)
    case $3 in
	on) text="canlock: on" ;;
	off) text="canlock: libcanlock $2 found, 3.3 or later is needed, off" ;;
	fail) text="cannot build canlock here (libcanlock $2 found, 3.3 or later is needed)" ;;
    esac
    case $line in
	*"$text"*) ;;
	*) echo "FAIL libcanlock $2 ${4:-}: configure said '$line', expected '$text'"; fail=1; return ;;
    esac
    if [ "$3" = fail ] && [ "$rc" -eq 0 ]; then
	echo "FAIL libcanlock $2 ${4:-}: configure exit status 0"; fail=1; return
    fi
    echo "ok   libcanlock $2 ${4:-}: $line"
}

check old 3.2 off
check older 2.9 off
check exact 3.3 on
check newer 3.4 on
check too-old-required 3.2 fail --with-canlock
[ "$fail" -eq 0 ] && echo "canlock probe checks passed"
exit "$fail"

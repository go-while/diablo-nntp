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

# mkstub DIR MAJOR MINOR [nolib]: DIR/inc/libcanlock-3/canlock.h and DIR/lib/libcanlock.a
# (with nolib: the library has no cl_get_key, so the link fails even where a real libcanlock
# sits in a system directory, as the first libcanlock in the -L directory is the one used)
mkstub() {
    mkdir -p "$1/inc/libcanlock-3" "$1/lib"
    printf '#define CL_API_MAJOR %s\n#define CL_API_MINOR %s\nint cl_get_key(void);\n' "$2" "$3" > "$1/inc/libcanlock-3/canlock.h"
    if [ "${4:-}" = nolib ]; then fn=cl_other_function; else fn=cl_get_key; fi
    printf 'int %s(void) { return 0; }\n' "$fn" > "$1/stub.c"
    $cc -c -o "$1/stub.o" "$1/stub.c"
    ar rc "$1/lib/libcanlock.a" "$1/stub.o"
}

# configure tries the package prefixes after the system directories, so a real libcanlock under
# one of them (FreeBSD's /usr/local) is found after the too-old stand-in and turns canlock on
# (correctly); the cases that expect "off" or an error cannot be shown on such a host
prefixed=0
for p in /usr/local /opt/local /opt/ooce; do
    [ -f "$p/include/libcanlock-3/canlock.h" ] && prefixed=1
done

# check NAME API-VERSION EXPECTED-TEXT [configure option]; NAME ending in -nolib: no cl_get_key in the library
check() {
    if [ "$prefixed" -eq 1 ] && [ "$3" != on ]; then
	echo "skip libcanlock $2 ${4:-}: a libcanlock is installed under a package prefix"
	return
    fi
    case $1 in *-nolib) lib=nolib ;; *) lib=lib ;; esac
    mkstub "$tmp/$1" "${2%.*}" "${2#*.}" "$lib"
    rc=0
    out=$(CC="$cc -isystem $tmp/$1/inc -L$tmp/$1/lib" ./configure -n ${4:-} 2>&1) || rc=$?
    line=$(printf '%s\n' "$out" | grep -i canlock | head -1)
    case $3 in
	on) text="canlock: on" ;;
	off) text="canlock: libcanlock API $2 found, 3.3 or later is needed, off" ;;
	fail) text="cannot build canlock here (libcanlock API $2 found, 3.3 or later is needed)" ;;
	nolink) text="canlock: libcanlock API $2 headers found but -lcanlock does not link, off" ;;
	nolinkfail) text="cannot build canlock here (libcanlock API $2 headers found but -lcanlock does not link)" ;;
    esac
    case $line in
	*"$text"*) ;;
	*) echo "FAIL libcanlock $2 ${4:-}: configure said '$line', expected '$text'"; fail=1; return ;;
    esac
    case $line in
	*"3.3 or later is needed"*)
	    case $3 in nolink*) echo "FAIL libcanlock $2 ${4:-}: header API $2 is not too old, but configure said '$line'"; fail=1; return ;; esac ;;
    esac
    if { [ "$3" = fail ] || [ "$3" = nolinkfail ]; } && [ "$rc" -eq 0 ]; then
	echo "FAIL libcanlock $2 ${4:-}: configure exit status 0"; fail=1; return
    fi
    echo "ok   libcanlock $2 ${4:-}: $line"
}

check old 3.2 off
check older 2.9 off
check exact 3.3 on
check newer 3.4 on
check too-old-required 3.2 fail --with-canlock
check nolib33-nolib 3.3 nolink
check nolib34-nolib 3.4 nolink
check nolib34-required-nolib 3.4 nolinkfail --with-canlock
[ "$fail" -eq 0 ] && echo "canlock probe checks passed"
exit "$fail"

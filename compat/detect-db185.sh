
#!/bin/sh
#
# Detect Berkeley DB 1.85 compatibility headers and libraries.
#
# Supports pkgsrc, OOCE, and common system installation paths.
# Outputs GNU Make variable assignments on success.
#
# Usage:
#   sh compat/detect-db185.sh
#   CC=gcc sh compat/detect-db185.sh
#

set -eu

CC=${CC:-cc}

# Optional overrides for unusual installations.
DB185_PREFIX=${DB185_PREFIX:-}
DB185_INCLUDEDIR=${DB185_INCLUDEDIR:-}
DB185_LIBDIR=${DB185_LIBDIR:-}

# Use a private temporary directory.
tmpdir=$(mktemp -d "${TMPDIR:-/tmp}/diablo-db185.XXXXXXXX")
trap 'rm -rf "$tmpdir"' 0
trap 'exit 1' 1 2 3 15

cat > "$tmpdir/test.c" <<'EOF'
#include <sys/types.h>
#include <fcntl.h>
#include <db_185.h>

int main(void)
{
    DB *db = dbopen("/nonexistent/diablo-db185-test",
                    O_RDONLY, 0, DB_BTREE, NULL);
    if (db != NULL)
        db->close(db);
    return 0;
}
EOF

# Header/library candidates, grouped by installation prefix.
prefixes="
/opt/local
/opt/ooce
/usr/local
/usr
"

if [ -n "$DB185_PREFIX" ]; then
    prefixes="$DB185_PREFIX $prefixes"
fi

for prefix in $prefixes; do

    # Prefer explicitly configured directories.
    if [ -n "$DB185_INCLUDEDIR" ]; then
        includedirs="$DB185_INCLUDEDIR"
    else
        includedirs="
$prefix/include/db5
$prefix/include/db4
$prefix/include
"
    fi


    if [ -n "$DB185_LIBDIR" ]; then
        libdirs="$DB185_LIBDIR"
    else
        libdirs="
$prefix/lib/amd64
$prefix/lib
$prefix/lib/64
"
    fi

    for incdir in $includedirs; do
        [ -f "$incdir/db_185.h" ] || continue

        for libdir in $libdirs; do
            [ -d "$libdir" ] || continue

            for lib in db5 db4 db; do

                # Test the header and library together.
                if "$CC" \
                    -I"$incdir" \
                    "$tmpdir/test.c" \
                    -L"$libdir" \
                    -Wl,-rpath,"$libdir" \
                    -l"$lib" \
                    -o "$tmpdir/test" \
                    > "$tmpdir/build.log" 2>&1
                then
                    echo "DB185_CPPFLAGS := -I$incdir"
                    echo "DB185_LDFLAGS := -L$libdir -Wl,-rpath,$libdir"
                    echo "DB185_LIBS := -l$lib"
                    exit 0
                fi

            done
        done
    done
done

echo "ERROR: No usable Berkeley DB 1.85 compatibility library found." >&2
echo "Searched pkgsrc, OOCE, and standard installation paths." >&2
exit 1


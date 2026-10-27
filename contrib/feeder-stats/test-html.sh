#!/bin/sh
# Check that the HTML written by feeder-stats.pl is well formed where it was
# not: the out/in ratio cell of an outgoing feed that has no incoming
# counterpart was written as "&#133;</tr>" (no </td>, and a Windows-1252
# character reference).  Usage: sh test-html.sh [path/to/feeder-stats.pl]
PL=${1:-$(dirname "$0")/feeder-stats.pl}
T=$(mktemp -d "${TMPDIR:-/tmp}/fstest.XXXXXX") || exit 2
trap 'rm -rf "$T"' EXIT

# Old perls have getopts.pl and timelocal.pl; newer ones no longer do.
if ! perl -e 'require "getopts.pl"; require "timelocal.pl"' 2>/dev/null; then
	mkdir "$T/lib"
	printf 'use Getopt::Std;\nsub Getopts { my ($spec) = @_; getopts ($spec); }\n1;\n' > "$T/lib/getopts.pl"
	printf 'use Time::Local;\n1;\n' > "$T/lib/timelocal.pl"
	PERL5LIB="$T/lib${PERL5LIB:+:$PERL5LIB}"; export PERL5LIB
fi
printf '#!/bin/sh\nexit 0\n' > "$T/doutq"
chmod 755 "$T/doutq"
FEEDER_STATS_QUEUECMD="$T/doutq"; export FEEDER_STATS_QUEUECMD

mkdir "$T/cfg" "$T/www"
printf 'label GLOBAL\nend\nlabel peera\n    inhost peera.example.com\nend\n' > "$T/cfg/dnewsfeeds"
: > "$T/cfg/diablo.hosts"
D=$(date +%Y%m%d)
TS=$(date '+%b %e %H:%M:%S')
{
	echo "$TS news diablo[100]: peera.example.com secs=60 ihave=0 chk=100 takethis=10 rec=10 acc=10 ref=1 precom=0 postcom=0 his=0 badmsgid=0 ifilthash=0 rej=1 ctl=0 spam=0 err=0 recbytes=1000 accbytes=1000 rejbytes=10 (1/sec)"
	# an outgoing feed that never fed us: its out/in ratio cannot be computed
	echo "$TS news newslink[200]: out.example.org:/news/dqueue/outonly.S00001 final secs=60 acc=7 dup=1 rej=1 tot=9 bytes=7000 (1/min) avpend=1.0 x"
} > "$T/news.notice"

perl "$PL" -d "$D" -C "$T/cfg" -l "$T/news.notice" -o "$T/out.html" -w "$T/www" > "$T/stdout" 2>&1 ||
	{ echo "FAIL: feeder-stats.pl failed"; cat "$T/stdout"; exit 1; }

fail=0
open=$(grep -o '<td[ >]' "$T/out.html" | wc -l)
close=$(grep -o '</td>' "$T/out.html" | wc -l)
if [ "$open" != "$close" ]; then
	echo "FAIL: $open <td> but $close </td>"; fail=1
fi
if grep -q '&#13[0-9];' "$T/out.html"; then
	echo "FAIL: Windows-1252 character reference in the HTML"; fail=1
fi
if ! grep -q '&hellip;' "$T/out.html"; then
	echo "FAIL: no ellipsis cell found (the fixture should produce one)"; fail=1
fi
[ $fail = 0 ] && echo "ok: feeder-stats HTML" || exit 1

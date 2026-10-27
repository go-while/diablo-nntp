#!/bin/sh
# feeder-stats.pl needed getopts.pl and timelocal.pl, which perl no longer ships (it uses the
# Getopt::Std and Time::Local modules now).  Run it without any stand-in for them.
# Usage: sh test-perl.sh [path/to/feeder-stats.pl]
LC_ALL=C; export LC_ALL
PL=${1:-$(dirname "$0")/feeder-stats.pl}
T=$(mktemp -d "${TMPDIR:-/tmp}/fstest.XXXXXX") || exit 2
trap 'rm -rf "$T"' EXIT
fail=0
bad() { echo "FAIL: $1"; fail=1; }

printf '#!/bin/sh\nexit 0\n' > "$T/doutq"
chmod 755 "$T/doutq"
FEEDER_STATS_QUEUECMD="$T/doutq"; export FEEDER_STATS_QUEUECMD
mkdir "$T/cfg" "$T/www"
printf 'label GLOBAL\nend\n' > "$T/cfg/dnewsfeeds"
: > "$T/cfg/diablo.hosts"
D=$(date +%Y%m%d)
TS=$(date '+%b %e %H:%M:%S')
echo "$TS news diablo[100]: peera.example.com secs=60 ihave=0 chk=10 takethis=10 rec=10 acc=10 ref=0 precom=0 postcom=0 his=0 badmsgid=0 ifilthash=0 rej=0 ctl=0 spam=0 err=0 recbytes=1000 accbytes=1000 rejbytes=0 (1/sec)" > "$T/news.notice"

# the options are read (getopts) and the day of the week is computed (timelocal)
perl "$PL" -h > "$T/help.txt" 2>&1
grep -q 'Usage:' "$T/help.txt" || { bad "-h does not print the usage"; cat "$T/help.txt"; }
perl "$PL" -t -d "$D" -C "$T/cfg" -l "$T/news.notice" -o "$T/out.txt" -w "$T/www" > "$T/stdout" 2>&1 ||
	{ bad "feeder-stats.pl failed"; cat "$T/stdout"; }
grep -q 'peera.example.com' "$T/out.txt" 2>/dev/null || bad "the report has no row for the feed"
[ -s "$T/www/index.html" ] || bad "no index page was written"
[ $fail = 0 ] && echo "ok: feeder-stats on current perl" || exit 1

#!/bin/sh
# Check the %Acc figures of feeder-stats.pl: the Total row was 0.00 for
# incoming feeds and a fraction (0.57 for 57%) for outgoing feeds, and a row
# could show more than 100.  Usage: sh test-totals.sh [path/to/feeder-stats.pl]
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
printf 'label GLOBAL\nend\n' > "$T/cfg/dnewsfeeds"
: > "$T/cfg/diablo.hosts"
D=$(date +%Y%m%d)
TS=$(date '+%b %e %H:%M:%S')
line() {	# line NAME CHK ACC
	echo "$TS news diablo[100]: $1 secs=60 ihave=0 chk=$2 takethis=$3 rec=$3 acc=$3 ref=0 precom=0 postcom=0 his=0 badmsgid=0 ifilthash=0 rej=0 ctl=0 spam=0 err=0 recbytes=1000 accbytes=1000 rejbytes=0 (1/sec)"
}
out() {		# out HOST ACC DUP REJ
	echo "$TS news newslink[200]: $1:/news/dqueue/q.S00001 final secs=60 acc=$2 dup=$3 rej=$4 tot=9 bytes=7000 (1/min) avpend=1.0 x"
}
{
	line peera 100 80	# 80% of what was offered
	line peerb 100 20	# 20%
	line peerc 100 101	# one more than was offered: shown as 100, not 101
	out feeda.example.org 7 2 1		# 7 of 10
	out feedb.example.org 3 0 0		# 3 of 3
} > "$T/news.notice"

perl "$PL" -t -d "$D" -C "$T/cfg" -l "$T/news.notice" -o "$T/out.txt" -w "$T/www" > "$T/stdout" 2>&1 ||
	{ echo "FAIL: feeder-stats.pl failed"; cat "$T/stdout"; exit 1; }

fail=0
check() {	# check PATTERN DESCRIPTION
	if ! grep -q -- "$1" "$T/out.txt"; then
		echo "FAIL: $2 (no match for: $1)"; fail=1
	fi
}
# incoming, by article: 201 accepted of 300 offered; peerc is capped at 100.00
check '^ *peerc  *101 100.00 ' 'row above 100 is capped'
check '^  *201 67.00 ' 'incoming Total %Acc'
# outgoing: 10 accepted of 13
check '^feeda.example.org  *7 70.00 ' 'outgoing row %Acc'
check '^  *10 76.92 ' 'outgoing Total %Acc'
if grep -q ' 100\.01 ' "$T/out.txt"; then echo "FAIL: a row shows more than 100"; fail=1; fi
[ $fail = 0 ] && echo "ok: feeder-stats %Acc" || { echo "--- output:"; sed -n '/^1\. /,/^2\. /p;/^5\. /,/^6\. /p' "$T/out.txt"; exit 1; }

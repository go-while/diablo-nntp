#!/bin/sh
# Check feeder-stats.pl -L (report feeds per dnewsfeeds label) on made-up
# log lines.  Usage: sh test-label.sh [path/to/feeder-stats.pl]
# Needs perl with Math::BigFloat; no news server, no network.
LC_ALL=C; export LC_ALL	# the fixtures use English month names, as syslog does
PL=${1:-$(dirname "$0")/feeder-stats.pl}
T=$(mktemp -d "${TMPDIR:-/tmp}/fstest.XXXXXX") || exit 2
trap 'rm -rf "$T"' EXIT
fail=0
check() {	# check FILE PATTERN DESCRIPTION
	if ! grep -q -- "$2" "$1"; then
		echo "FAIL: $3 (no match for: $2)"; fail=1
	fi
}
nocheck() {	# nocheck FILE PATTERN DESCRIPTION
	if grep -q -- "$2" "$1"; then
		echo "FAIL: $3 (found: $2)"; fail=1
	fi
}

# Old perls have getopts.pl and timelocal.pl; newer ones no longer do.
if ! perl -e 'require "getopts.pl"; require "timelocal.pl"' 2>/dev/null; then
	mkdir "$T/lib"
	cat > "$T/lib/getopts.pl" <<'EOF'
use Getopt::Std;
sub Getopts { my ($spec) = @_; getopts ($spec); }
1;
EOF
	cat > "$T/lib/timelocal.pl" <<'EOF'
use Time::Local;
1;
EOF
	PERL5LIB="$T/lib${PERL5LIB:+:$PERL5LIB}"; export PERL5LIB
fi

mkdir "$T/cfg" "$T/www"
cat > "$T/cfg/dnewsfeeds" <<'EOF'
label GLOBAL
end
label peera
    inhost	peera.example.com
    inhost	192.0.2.0/28
    hostname	out.peera.example.org
end
label peerb
    host	peerb.example.net
end
label wild
    inhost	*.wild.example
end
label peerv6
    inhost	2001:db8::/32
end
EOF
: > "$T/cfg/diablo.hosts"

D=$(date +%Y%m%d)
TS=$(date '+%b %e %H:%M:%S')
line() {	# line NAME ACC
	echo "$TS news diablo[100]: $1 secs=60 ihave=0 chk=100 takethis=$2 rec=$2 acc=$2 ref=1 precom=0 postcom=0 his=0 badmsgid=0 ifilthash=0 rej=1 ctl=0 spam=0 err=0 recbytes=1000 accbytes=1000 rejbytes=10 (1/sec)"
}
{
	line peera.example.com 10
	line 192.0.2.5 20
	line peerb.example.net 30
	line host.wild.example 40
	line 2001:db8:1::5 50
	line 198.51.100.9 60
	echo "$TS news newslink[200]: out.peera.example.org:/news/dqueue/peera.S00001 final secs=60 acc=7 dup=1 rej=1 tot=9 bytes=7000 (1/min) avpend=1.0 x"
	echo "$TS news newslink[201]: 192.0.2.77:/news/dqueue/peerb.S00002 final secs=60 acc=3 dup=0 rej=0 tot=3 bytes=3000 (1/min) avpend=1.0 x"
} > "$T/news.notice"

# The script runs doutq for the queue table; use a stand-in that lists no queues.
printf '#!/bin/sh\nexit 0\n' > "$T/doutq"
chmod 755 "$T/doutq"
FEEDER_STATS_QUEUECMD="$T/doutq"; export FEEDER_STATS_QUEUECMD

run() {	# run OUTFILE [options]
	out=$1; shift
	perl "$PL" -t -d "$D" -C "$T/cfg" -l "$T/news.notice" -o "$out" -w "$T/www" "$@" > "$T/stdout" 2>&1 ||
		{ echo "FAIL: feeder-stats.pl failed"; cat "$T/stdout"; exit 1; }
}

run "$T/plain.txt"
run "$T/label.txt" -L

# Without -L the hosts are separate rows, as before.
check "$T/plain.txt" 'peera.example.com' 'plain: host name row'
check "$T/plain.txt" '192.0.2.5' 'plain: IP row'
nocheck "$T/plain.txt" '^ *[0-9]* *peera ' 'plain: no label row'

# With -L they are grouped by label: peera = host name + CIDR (10 + 20).
check "$T/label.txt" 'peera .* 30 ' '-L: peera counts both hosts (30 articles)'
check "$T/label.txt" 'peerb .* 30 ' '-L: peerb from the host option'
check "$T/label.txt" 'wild .* 40 ' '-L: wildcard inhost'
check "$T/label.txt" 'peerv6 .* 50 ' '-L: IPv6 CIDR'
check "$T/label.txt" '198.51.100.9' '-L: unknown host keeps its name'
nocheck "$T/label.txt" 'peera.example.com' '-L: host name replaced by label'
nocheck "$T/label.txt" '192.0.2.5' '-L: IP replaced by label'
# Outgoing: the label from the batch name, not the host the feed connects to.
check "$T/label.txt" 'peera .* 7 ' '-L: outgoing peera'
check "$T/label.txt" 'peerb .* 3 ' '-L: outgoing peerb'
nocheck "$T/label.txt" '192.0.2.77' '-L: outgoing host replaced by label'

if [ $fail = 0 ]; then echo "ok: feeder-stats -L"; else
	echo "--- output with -L:"; cat "$T/label.txt"; exit 1; fi

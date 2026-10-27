#!/bin/sh
# Check the Top1000 support of feeder-stats.pl: top1000-fetch.sh gets and
# keeps the list, -T points the script at it, the rank shows in the HTML for a
# feed whose dnewsfeeds alias is in the list, and unlisted hosts are silent.
# Usage: sh test-top1000.sh [path/to/feeder-stats.pl]
HERE=$(cd "$(dirname "$0")" && pwd)
PL=${1:-$HERE/feeder-stats.pl}
T=$(mktemp -d "${TMPDIR:-/tmp}/fstest.XXXXXX") || exit 2
trap 'rm -rf "$T"' EXIT
fail=0
bad() { echo "FAIL: $1"; fail=1; }

# A list in the format of https://top1000.anthologeek.net/top1000.txt
{
	printf '\tRank\tWeight\tPathname\n========================================\n'
	printf '\t1\t45.748479\tfeeder.usenetexpress.com\n\t2\t24.596312\tnews.example.net\n'
	i=3; while [ $i -le 12 ]; do printf '\t%d\t1.0\tfiller%d.example.org\n' $i $i; i=$((i + 1)); done
} > "$T/list.txt"

# --- top1000-fetch.sh
sh "$HERE/top1000-fetch.sh" "$T/dir" "file://$T/list.txt" || bad "fetch failed"
cmp -s "$T/list.txt" "$T/dir/current" || bad "list not stored as DIR/current"
echo "not a list" > "$T/junk.txt"
touch -t 200001010000 "$T/dir/current"
sh "$HERE/top1000-fetch.sh" "$T/dir" "file://$T/junk.txt" 2>/dev/null && bad "junk accepted"
cmp -s "$T/list.txt" "$T/dir/current" || bad "old list not kept after a bad download"
sh "$HERE/top1000-fetch.sh" "$T/dir" "file://$T/missing.txt" 2>/dev/null && bad "missing file accepted"

# --- feeder-stats.pl
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
cat > "$T/cfg/dnewsfeeds" <<'EOF'
label GLOBAL
end
label uex
    inhost 198.51.100.7
    alias feeder.usenetexpress.com
end
label other
    inhost 203.0.113.9
    alias nowhere.example.com
end
EOF
: > "$T/cfg/diablo.hosts"
D=$(date +%Y%m%d)
TS=$(date '+%b %e %H:%M:%S')
for n in 198.51.100.7 203.0.113.9; do
	echo "$TS news diablo[1]: $n secs=60 ihave=0 chk=100 takethis=10 rec=10 acc=10 ref=0 precom=0 postcom=0 his=0 badmsgid=0 ifilthash=0 rej=0 ctl=0 spam=0 err=0 recbytes=1000 accbytes=1000 rejbytes=0 (1/sec)"
done > "$T/news.notice"

run() {	# run [options]
	perl "$PL" -T "$T/dir" -d "$D" -C "$T/cfg" -l "$T/news.notice" -o "$T/out.html" -w "$T/www" -s feeder.usenetexpress.com "$@" > "$T/stdout" 2>&1 ||
		{ bad "feeder-stats.pl failed"; cat "$T/stdout"; }
}
run
grep -q '198.51.100.7</td> <td align="right">&nbsp;#1</td>' "$T/out.html" || bad "rank #1 not shown for the feed with a listed alias"
grep -q '203.0.113.9</td> <td align="right">&nbsp;</td>' "$T/out.html" || bad "an unlisted feed should have an empty rank cell"
grep -q '(Top1000 #1)' "$T/out.html" || bad "rank of the server itself (-s) not in the title"
grep -q 'Pos1000' "$T/stdout" && bad "'Pos1000' messages without -v"
run -v
grep -q 'Pos1000 is empty for host 203.0.113.9' "$T/stdout" || bad "no 'Pos1000' message with -v"

[ $fail = 0 ] && echo "ok: feeder-stats Top1000" || exit 1

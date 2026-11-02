#!/bin/sh
# feeder-stats.pl must tell the process that wrote a syslog line from the name of the host:
# on a host called diablo1 (or diablo-reader, or one with "reader" in its name) every line,
# dnewslink's too, has "diablo" in it, and the outgoing feeds vanished from the report.
# Usage: sh test-host.sh [path/to/feeder-stats.pl]
# Needs perl with Math::BigFloat; no news server, no network.
LC_ALL=C; export LC_ALL
PL=${1:-$(dirname "$0")/feeder-stats.pl}
T=$(mktemp -d "${TMPDIR:-/tmp}/fstest.XXXXXX") || exit 2
trap 'rm -rf "$T"' EXIT
fail=0

printf '#!/bin/sh\nexit 0\n' > "$T/doutq"
chmod 755 "$T/doutq"
FEEDER_STATS_QUEUECMD="$T/doutq"; export FEEDER_STATS_QUEUECMD
mkdir "$T/cfg" "$T/www"
printf 'label GLOBAL\nend\n' > "$T/cfg/dnewsfeeds"
: > "$T/cfg/diablo.hosts"
D=$(date +%Y%m%d)
TS=$(date '+%b %e %H:%M:%S')

for HOST in news diablo1 diablo-reader.example.org reader1; do
	{
		echo "$TS $HOST diablo[100]: peera.example.com secs=60 ihave=0 chk=100 takethis=10 rec=10 acc=10 ref=1 precom=0 postcom=0 his=0 badmsgid=0 ifilthash=0 rej=1 ctl=0 spam=0 err=0 recbytes=1000 accbytes=1000 rejbytes=10 (1/sec)"
		echo "$TS $HOST newslink[200]: out.peerb.example.net:/news/dqueue/peerb.S00001 final secs=60 acc=7 dup=1 rej=1 tot=9 bytes=7000 (1/min) avpend=1.0 x"
	} > "$T/news.notice"
	perl "$PL" -t -d "$D" -C "$T/cfg" -l "$T/news.notice" -o "$T/out.txt" -w "$T/www" > "$T/stdout" 2>&1 ||
		{ echo "FAIL: feeder-stats.pl failed (host $HOST)"; cat "$T/stdout"; fail=1; continue; }
	grep -q 'peera.example.com' "$T/out.txt" || { echo "FAIL: host $HOST: no incoming row for peera.example.com"; fail=1; }
	grep -q 'out.peerb.example.net' "$T/out.txt" || { echo "FAIL: host $HOST: no outgoing row for out.peerb.example.net"; fail=1; }
done

[ $fail = 0 ] && echo "ok: feeder-stats on hosts with diablo and reader in their names" || exit 1

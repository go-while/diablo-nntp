#!/bin/sh
# feeder-stats.pl must tell the process that wrote a syslog line from the name of the host:
# on a host called diablo1 (or diablo-reader, or one with "reader" in its name) every line,
# dnewslink's too, has "diablo" in it, and the outgoing feeds vanished from the report.
# The process is the "name[pid]:" tag, also when the syslog writes no host (then the
# destination of dnewslink's line must not be taken for the process), after Solaris'
# "[ID n facility.level]" and after a time with fractions of a second.  The reader lines
# of dreaderd and its children (reader, feed, ...) go to the reader activity.
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
# one date call, so that the day and the time agree at midnight
NOW=$(date '+%Y%m%d|%b %e %H:%M:%S')
D=${NOW%%|*}
TS=${NOW#*|}

# <name of the case>|<what comes before the tag>|<dnewslink's tag>|<after the tag>|<reader's tag>
while IFS='|' read -r NAME PRE NL ID RD; do
	{
		echo "$PRE diablo[100]:$ID peera.example.com secs=60 ihave=0 chk=100 takethis=10 rec=10 acc=10 ref=1 precom=0 postcom=0 his=0 badmsgid=0 ifilthash=0 rej=1 ctl=0 spam=0 err=0 recbytes=1000 accbytes=1000 rejbytes=10 (1/sec)"
		echo "$PRE $NL[200]:$ID out.peerb.example.net:/news/dqueue/peerb.S00001 final secs=60 acc=7 dup=1 rej=1 tot=9 bytes=7000 (1/min) avpend=1.0 x"
		echo "$PRE $RD[300]:$ID closed from client.example.org (192.0.2.7:4000) groups=3 arts=4 bytes=5000 posts=0 postbytes=0 postfail=0 secs=60"
	} > "$T/news.notice"
	perl "$PL" -t -d "$D" -C "$T/cfg" -l "$T/news.notice" -o "$T/out.txt" -w "$T/www" > "$T/stdout" 2>&1 ||
		{ echo "FAIL: feeder-stats.pl failed ($NAME)"; cat "$T/stdout"; fail=1; continue; }
	grep -q 'peera.example.com' "$T/out.txt" || { echo "FAIL: $NAME: no incoming row for peera.example.com"; fail=1; }
	grep -q 'out.peerb.example.net' "$T/out.txt" || { echo "FAIL: $NAME: no outgoing row for out.peerb.example.net"; fail=1; }
	grep -q 'Summary of Reader Activity' "$T/out.txt" || { echo "FAIL: $NAME: no reader activity ($RD)"; fail=1; }
done <<CASES
host news|$TS news|newslink||dreaderd
host diablo1|$TS diablo1|newslink||dreaderd
host diablo-reader.example.org|$TS diablo-reader.example.org|newslink||dreaderd
host reader1|$TS reader1|newslink||dreaderd
host reader1, feed child|$TS reader1|newslink||feed
host diablo|$TS diablo|newslink||dreaderd
host diablo1, dnewslink|$TS diablo1|dnewslink||dreaderd
no host|$TS|newslink||dreaderd
no host, dnewslink|$TS|dnewslink||dreaderd
host diablo1, Solaris [ID]|$TS diablo1|newslink| [ID 123456 news.notice]|dreaderd
no host, Solaris [ID]|$TS|newslink| [ID 123456 news.notice]|dreaderd
host diablo1, fractional seconds|$TS.123456 diablo1|newslink||dreaderd
host diablo1, reader-dns child|$TS diablo1|newslink||reader-dns
CASES

[ $fail = 0 ] && echo "ok: feeder-stats on hosts with diablo and reader in their names, and without a host" || exit 1

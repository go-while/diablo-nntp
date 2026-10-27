#!/bin/sh
# Fetch the Top1000 list that feeder-stats.pl reads (the file "current" in
# its Top1000 directory, see the -T option).  Usage:
#
#	top1000-fetch.sh DIR [URL]
#
# URL defaults to the list of last month on https://top1000.anthologeek.net/.
# The list is only fetched when DIR/current is missing or more than a day old,
# and the old file is kept when the download fails or does not look like the
# list (a tab, a rank, a weight and a path name per line).  Needs curl, fetch
# (FreeBSD) or wget.  Exit status: 0 list is current, 1 could not be updated.

DIR=${1:?usage: top1000-fetch.sh DIR [URL]}
URL=${2:-https://top1000.anthologeek.net/top1000.txt}
FILE=$DIR/current

mkdir -p "$DIR" || exit 1
if [ -s "$FILE" ] && [ -z "$(find "$FILE" -mtime +0 -print 2>/dev/null)" ]; then
	exit 0
fi

TMP=$(mktemp "$DIR/current.XXXXXX") || exit 1
trap 'rm -f "$TMP"' EXIT

if command -v curl >/dev/null 2>&1; then
	curl -fsS -m 60 -o "$TMP" "$URL"
elif command -v fetch >/dev/null 2>&1; then
	fetch -q -T 60 -o "$TMP" "$URL"
elif command -v wget >/dev/null 2>&1; then
	wget -q -T 60 -O "$TMP" "$URL"
else
	echo "top1000-fetch.sh: need curl, fetch or wget" >&2
	exit 1
fi || { echo "top1000-fetch.sh: cannot fetch $URL" >&2; exit 1; }

# Ten list lines at least, or it is not the list.
n=$(grep -c '^[[:space:]][[:space:]]*[0-9][0-9]*[[:space:]][[:space:]]*[0-9.][0-9.]*[[:space:]][[:space:]]*[^[:space:]][^[:space:]]*$' "$TMP")
if [ "${n:-0}" -lt 10 ]; then
	echo "top1000-fetch.sh: $URL does not look like a Top1000 list; keeping the old file" >&2
	exit 1
fi
chmod 644 "$TMP"
mv "$TMP" "$FILE"

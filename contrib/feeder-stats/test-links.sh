#!/bin/sh
# feeder-stats.pl linked a download address that no longer resolves
# (info.news.surf.net) and the old Top1000 site.  Usage: sh test-links.sh [feeder-stats.pl]
LC_ALL=C; export LC_ALL	# the fixtures use English month names, as syslog does
PL=${1:-$(dirname "$0")/feeder-stats.pl}
T=$(mktemp -d "${TMPDIR:-/tmp}/fstest.XXXXXX") || exit 2
trap 'rm -rf "$T"' EXIT
if ! perl -e 'require "getopts.pl"; require "timelocal.pl"' 2>/dev/null; then
	mkdir "$T/lib"
	printf 'use Getopt::Std;\nsub Getopts { my ($spec) = @_; getopts ($spec); }\n1;\n' > "$T/lib/getopts.pl"
	printf 'use Time::Local;\n1;\n' > "$T/lib/timelocal.pl"
	PERL5LIB="$T/lib${PERL5LIB:+:$PERL5LIB}"; export PERL5LIB
fi
fail=0
perl "$PL" -h > "$T/help.txt" 2>&1
if grep -q 'surf\.net' "$T/help.txt" "$PL"; then
	echo "FAIL: the dead info.news.surf.net address is still used"; fail=1
fi
if ! grep -q 'github.com/jrehmer/diablo-nntp/tree/main/contrib/feeder-stats' "$T/help.txt"; then
	echo "FAIL: -h does not show the project address"; fail=1
fi
if ! grep -q '^\$Top1000Url = "https://top1000.anthologeek.net/";' "$PL"; then
	echo "FAIL: Top1000 link is not https://top1000.anthologeek.net/"; fail=1
fi
[ $fail = 0 ] && echo "ok: feeder-stats links" || exit 1

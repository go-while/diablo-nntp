#! /bin/sh
#
# decrypt-aes.sh - read back what dreaderd encrypted with a vserverdef cryptpw:
# X-Trace: (rfc5537 no) and the posting-host and posting-account of
# Injection-Info: (rfc5537 yes).  Each is "C=<base64>" or "G=<gen>,C=<base64>",
# AES-256-CBC in the openssl enc format, so this is the same as
#
#	echo '<base64>' | openssl enc -d -aes-256-cbc -md sha256 -a -A -pass pass:<cryptpw>
#
# The article is read from stdin, or with -m from this server's spool
# (dreadart -h).  Without a password every cryptpw in dreader.access is
# tried, so run it as news (or root) on the server.  Plain sh, sed and
# openssl: works on Linux, the BSDs, macOS and illumos.

usage() {
	exec >&2
	echo "usage: decrypt-aes.sh [-m '<message-id>'] [-a dreader.access] [password]"
	echo
	echo "Decrypts the encrypted parts of the X-Trace: and Injection-Info:"
	echo "headers dreaderd wrote with a cryptpw."
	echo
	echo "  -m '<message-id>'  read the article from the spool (dreadart -h);"
	echo "                     without -m it is read from stdin"
	echo "  -a file            the dreader.access to take the cryptpw from"
	echo "                     (default: <dpath lib>/dreader.access)"
	echo "  password           pass:<cryptpw>, env:<VAR> or file:<path>; without"
	echo "                     it every cryptpw in dreader.access is tried."
	echo "                     The password is the P= part of a"
	echo "                     \"G=gen,P=password\" cryptpw.  env: and file:"
	echo "                     keep it out of ps."
	echo
	echo "examples: decrypt-aes.sh -m '<abc\$1@example.com>'"
	echo "          decrypt-aes.sh pass:secret < article"
	exit 1
}

DBIN=$(dirname "$0")
MSGID=
ACCESS=
while getopts m:a:h opt; do
	case $opt in
	m) MSGID=$OPTARG ;;
	a) ACCESS=$OPTARG ;;
	*) usage ;;
	esac
done
shift $((OPTIND - 1))

PASS=
case $# in
0) ;;
1)
	case $1 in
	pass:*|env:*|file:*|fd:*|stdin) PASS=$1 ;;
	*) usage ;;
	esac
	;;
*) usage ;;
esac

command -v openssl >/dev/null 2>&1 || { echo "decrypt-aes.sh: openssl not found" >&2; exit 1; }

# a diablo program: next to this script (in dbin), in the PATH, or /news/dbin
dprog() {
	if [ -x "$DBIN/$1" ]; then
		echo "$DBIN/$1"
	elif command -v "$1" >/dev/null 2>&1; then
		command -v "$1"
	elif [ -x "/news/dbin/$1" ]; then
		echo "/news/dbin/$1"
	else
		return 1
	fi
}

# without a password: every cryptpw of dreader.access, one per line
PWLIST=
if [ -z "$PASS" ]; then
	if [ -z "$ACCESS" ]; then
		lib=
		dp=$(dprog dpath) && lib=$("$dp" lib 2>/dev/null)
		ACCESS=${lib:-/news}/dreader.access
	fi
	if [ ! -r "$ACCESS" ]; then
		echo "decrypt-aes.sh: cannot read $ACCESS (run as news, or give -a or a password)" >&2
		exit 1
	fi
	for v in $(sed -n -e 's/^[[:space:]]*cryptpw[[:space:]][[:space:]]*\([^[:space:]#]*\).*/\1/p' "$ACCESS"); do
		pw=$v
		old_ifs=$IFS
		IFS=,
		for part in $v; do
			case $part in
			P=*) pw=${part#P=} ;;
			esac
		done
		IFS=$old_ifs
		PWLIST="$PWLIST$pw
"
	done
	if [ -z "$PWLIST" ]; then
		echo "decrypt-aes.sh: no cryptpw in $ACCESS" >&2
		exit 1
	fi
fi

# printable: the plain text has no control characters (a wrong password
# can give valid padding by chance, but not readable text)
printable() {
	[ -z "$(printf '%s' "$1" | LC_ALL=C tr -d '[:print:]\200-\377')" ]
}

# try <base64>: the plain text on stdout, status 1 if no password fits
try() {
	if [ -n "$PASS" ]; then
		d=$(printf '%s\n' "$1" | openssl enc -d -aes-256-cbc -md sha256 -a -A -pass "$PASS" 2>/dev/null) &&
			printable "$d" && printf '%s' "$d"
		return
	fi
	printf '%s' "$PWLIST" | {
		while IFS= read -r pw; do
			[ -n "$pw" ] || continue
			if d=$(printf '%s\n' "$1" | DECRYPT_AES_PW=$pw openssl enc -d -aes-256-cbc -md sha256 -a -A -pass env:DECRYPT_AES_PW 2>/dev/null) &&
			    printable "$d"; then
				printf '%s' "$d"
				exit 0
			fi
		done
		exit 1
	}
}

# decrypt <label> <value>: value is "C=<b64>", "G=<gen>,C=<b64>" or plain text
decrypt() {
	gen= c=
	old_ifs=$IFS
	IFS=,
	for part in $2; do
		case $part in
		G=*) gen=${part#G=} ;;
		C=*|DXC=*) c=${part#*C=} ;;
		esac
	done
	IFS=$old_ifs
	if [ -z "$c" ]; then
		echo "$1: $2 (not encrypted)"
		return 0
	fi
	if d=$(try "$c"); then
		if [ -n "$gen" ]; then
			echo "$1: $d (generation $gen)"
		else
			echo "$1: $d"
		fi
		return 0
	fi
	echo "$1: cannot decrypt (wrong cryptpw?)" >&2
	return 1
}

# the article: from the spool with -m, else stdin
article() {
	if [ -n "$MSGID" ]; then
		da=$(dprog dreadart) || { echo "decrypt-aes.sh: dreadart not found" >&2; exit 1; }
		"$da" -h "$MSGID" 2>/dev/null
	else
		cat
	fi
}

TAB=$(printf '\t')
TMP=$(mktemp "${TMPDIR:-/tmp}/decrypt-aes.XXXXXX") || exit 1
trap 'rm -f "$TMP"' EXIT
trap 'exit 1' HUP INT TERM
if ! article > "$TMP" || [ ! -s "$TMP" ]; then
	if [ -n "$MSGID" ]; then
		echo "decrypt-aes.sh: $MSGID is not in this server's spool" >&2
	else
		echo "decrypt-aes.sh: no article on stdin" >&2
	fi
	exit 1
fi
[ -n "$MSGID" ] && echo "Message-ID: $MSGID"

# the header only, CR removed, folded lines joined
tr -d '\r' < "$TMP" | sed -e '/^$/q' | sed -e "s/^[ $TAB][ $TAB]*/ /" |
	sed -e ':a' -e '$!N' -e 's/\n / /' -e 'ta' -e 'P' -e 'D' | {
	found=0 rc=0
	while IFS= read -r line; do
		name=${line%%:*}
		val=${line#*:}
		case $(printf '%s' "$name" | tr 'A-Z' 'a-z') in
		x-trace)
			val=$(printf '%s' "$val" | sed -e "s/^[ $TAB]*//" -e "s/[ $TAB]*\$//")
			found=1
			decrypt "X-Trace" "$val" || rc=1
			;;
		injection-info)
			old_ifs=$IFS
			IFS=';'
			set -f
			for p in $val; do
				IFS=$old_ifs
				p=$(printf '%s' "$p" | sed -e "s/^[ $TAB]*//" -e "s/[ $TAB]*\$//")
				key=${p%%=*}
				v=$(printf '%s' "${p#*=}" | sed -e 's/^"//' -e 's/"$//' -e 's/\\\(.\)/\1/g')
				case $key in
				posting-host|posting-account)
					found=1
					decrypt "Injection-Info $key" "$v" || rc=1
					;;
				esac
			done
			set +f
			IFS=$old_ifs
			;;
		esac
	done
	if [ $found = 0 ]; then
		echo "decrypt-aes.sh: no X-Trace: or Injection-Info: posting-host/posting-account found" >&2
		exit 1
	fi
	exit $rc
}

#! /bin/sh
#
# decrypt-aes.sh - read back what dreaderd encrypted with a vserverdef cryptpw:
# X-Trace: (rfc5537 no) and the posting-host and posting-account of
# Injection-Info: (rfc5537 yes).  Each is "C=<base64>" or "G=<gen>,C=<base64>",
# AES-256-CBC in the openssl enc format, so this is the same as
#
#	echo '<base64>' | openssl enc -d -aes-256-cbc -md sha256 -a -A -pass pass:<cryptpw>
#
# Feed it an article, its header, or just the header lines on stdin.  Plain
# sh, sed and openssl: works on Linux, the BSDs, macOS and illumos.

usage() {
	exec >&2
	echo "usage: decrypt-aes.sh pass:<cryptpw> < article"
	echo "       decrypt-aes.sh env:<VAR> < article     (the cryptpw in \$VAR)"
	echo "       decrypt-aes.sh file:<path> < article   (the cryptpw in a file)"
	echo
	echo "Decrypts the encrypted parts of the X-Trace: and Injection-Info:"
	echo "headers dreaderd wrote with a cryptpw.  The password is the P= part"
	echo "of a \"G=gen,P=password\" cryptpw.  env: and file: keep it out of ps."
	exit 1
}

[ $# = 1 ] || usage
case $1 in
pass:*|env:*|file:*|fd:*|stdin) PASS=$1 ;;
*) usage ;;
esac

command -v openssl >/dev/null 2>&1 || { echo "decrypt-aes.sh: openssl not found" >&2; exit 1; }

TAB=$(printf '\t')

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
	if d=$(printf '%s\n' "$c" | openssl enc -d -aes-256-cbc -md sha256 -a -A -pass "$PASS" 2>/dev/null); then
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

# the header only, CR removed, folded lines joined
tr -d '\r' | sed -e '/^$/q' | sed -e "s/^[ $TAB][ $TAB]*/ /" |
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

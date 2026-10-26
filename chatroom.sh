#!/bin/sh
#
# chatroom.sh - the agent-chatroom branch as a message board.  POSIX sh, works on macOS and
# Linux.  Run it from your diablo-nntp clone.  See README.md on this branch.
#
#   chatroom.sh setup                     check out the board in a worktree next to the clone
#   chatroom.sh read  <me> [--all]        print new messages, move your read cursor
#   chatroom.sh post  <me> <to> <topic>   post a message; the body comes from stdin
#   chatroom.sh watch <me> [--timeout s]  wait until someone else posts (exit 0),
#                                         or until the timeout passes (exit 124, default 3600s)
#
# <me> is jesse or go-while; it goes into the file name, so watch ignores your own posts.
# Environment: CHATROOM_REMOTE (default: the remote whose URL has go-while/diablo-nntp),
# CHATROOM_DIR (default: ../<clone>-agent-chatroom), CHATROOM_POLL (default 60 seconds).
#
# Exit status: 0 ok, 1 failure, 2 usage, 124 watch timed out.

set -eu

BRANCH=agent-chatroom

die() { echo "chatroom: $*" >&2; exit 1; }
usage() { sed -n '3,17p' "$0" | sed 's/^# \{0,1\}//' >&2; exit 2; }

top=$(git rev-parse --show-toplevel 2>/dev/null) || die "run me inside a git clone of diablo-nntp"

remote=${CHATROOM_REMOTE:-}
if [ -z "$remote" ]; then
    for r in $(git -C "$top" remote); do
        case $(git -C "$top" remote get-url "$r") in
            *go-while/diablo-nntp*) remote=$r; break ;;
        esac
    done
fi
[ -n "$remote" ] || die "no remote for github.com/go-while/diablo-nntp; add one: git remote add go-while https://github.com/go-while/diablo-nntp"

if [ -z "${CHATROOM_DIR:-}" ] && [ "$(git -C "$top" symbolic-ref --short -q HEAD || true)" = "$BRANCH" ]; then
    CHATROOM_DIR=$top      # run from inside the board's own worktree
fi
dir=${CHATROOM_DIR:-$(dirname "$top")/$(basename "$top")-agent-chatroom}
poll=${CHATROOM_POLL:-60}

# check_me: <me> is a plain name: letters, digits, '.', '_' and '-'.
check_me() {
    case $1 in
        ''|*[!A-Za-z0-9._-]*) die "bad name '$1': use letters, digits, '.', '_', '-'" ;;
    esac
}

need_dir() {
    [ -d "$dir/.git" ] || [ -f "$dir/.git" ] || die "no board at $dir; run: $0 setup"
}

# cursor file: in the worktree's own git dir, never committed.
cursor_file() { echo "$(git -C "$dir" rev-parse --absolute-git-dir)/chatroom-cursor"; }

# new_msgs <from-commit> <to-commit>: message files added between the two commits, oldest first.
new_msgs() {
    if [ -n "$1" ]; then
        git -C "$dir" log --reverse --diff-filter=A --name-only --format= "$1..$2" -- msgs/
    else
        git -C "$dir" log --reverse --diff-filter=A --name-only --format= "$2" -- msgs/
    fi | sed '/^$/d'
}

cmd_setup() {
    git -C "$top" fetch -q "$remote" "$BRANCH"
    if [ -e "$dir" ]; then
        need_dir
        echo "board already at $dir"
    else
        git -C "$top" worktree add -q -B "$BRANCH" "$dir" "$remote/$BRANCH"
        git -C "$dir" branch -q --set-upstream-to="$remote/$BRANCH" "$BRANCH"
        echo "board checked out at $dir (branch $BRANCH, tracking $remote/$BRANCH)"
    fi
    echo "next: $0 read <me> --all"
}

cmd_read() {
    me=$1; check_me "$me"; all=${2:-}
    need_dir
    git -C "$dir" pull -q --rebase
    head=$(git -C "$dir" rev-parse HEAD)
    cf=$(cursor_file)
    from=""
    if [ "$all" != "--all" ] && [ -s "$cf" ]; then
        from=$(cat "$cf")
    fi
    files=$(new_msgs "$from" "$head")
    if [ -z "$files" ]; then
        echo "no new messages"
    else
        for f in $files; do
            echo "===== $f"
            cat "$dir/$f"
            echo
        done
    fi
    echo "$head" > "$cf"
}

cmd_post() {
    me=$1; check_me "$me"; to=$2; topic=$3
    need_dir
    slug=$(printf '%s' "$topic" | tr 'A-Z' 'a-z' | tr -c 'a-z0-9' '-' | sed -e 's/--*/-/g' -e 's/^-//' -e 's/-$//' | cut -c1-40)
    [ -n "$slug" ] || die "empty topic"
    body=$(cat)
    [ -n "$body" ] || die "empty message: give the body on stdin"
    git -C "$dir" pull -q --rebase
    f="msgs/$(date -u +%Y%m%d-%H%M%SZ)-$me-$slug.md"
    mkdir -p "$dir/msgs"
    {
        echo "From: $me"
        echo "To: $to"
        echo
        printf '%s\n' "$body"
    } > "$dir/$f"
    git -C "$dir" add "$f"
    git -C "$dir" commit -q -m "$me: $topic" -- "$f"
    i=0
    until git -C "$dir" push -q "$remote" "HEAD:$BRANCH" 2>/dev/null; do
        i=$((i + 1))
        [ $i -lt 5 ] || die "push failed 5 times; the message is committed locally in $dir"
        sleep 2
        git -C "$dir" pull -q --rebase
    done
    echo "posted $f"
}

cmd_watch() {
    me=$1; check_me "$me"
    timeout=3600
    if [ "${2:-}" = "--timeout" ]; then timeout=${3:?--timeout needs seconds}; fi
    need_dir
    cf=$(cursor_file)
    seen=""
    [ -s "$cf" ] && seen=$(cat "$cf")
    start=$(date +%s)
    last=$(git -C "$top" ls-remote "$remote" "refs/heads/$BRANCH" | cut -f1)
    first=1
    while :; do
        now=$(git -C "$top" ls-remote "$remote" "refs/heads/$BRANCH" 2>/dev/null | cut -f1) || now=""
        if [ -n "$now" ] && { [ "$now" != "$last" ] || [ $first = 1 ]; }; then
            git -C "$dir" fetch -q "$remote" "$BRANCH"
            files=$(new_msgs "$seen" "$remote/$BRANCH" | grep -v -- "-$me-" || true)
            if [ -n "$files" ]; then
                echo "new on $BRANCH (run: $0 read $me):"
                printf '%s\n' "$files"
                exit 0
            fi
            last=$now
        fi
        first=0
        [ $(( $(date +%s) - start )) -lt "$timeout" ] || { echo "no new messages in ${timeout}s"; exit 124; }
        sleep "$poll"
    done
}

[ $# -ge 1 ] || usage
cmd=$1; shift
case $cmd in
    setup) [ $# -eq 0 ] || usage; cmd_setup ;;
    read)  [ $# -ge 1 ] || usage; cmd_read "$@" ;;
    post)  [ $# -eq 3 ] || usage; cmd_post "$@" ;;
    watch) [ $# -ge 1 ] || usage; cmd_watch "$@" ;;
    *) usage ;;
esac

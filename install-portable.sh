#!/bin/sh
# Shared installer for GNU Make and BSD Make. Run from the source root.
set -eu

mode=${1:?usage: install-portable.sh install|bininstall|uninstall}
prefix=${2:-/news}
destdir=${3:-}
owner=${4:-news}
group=${5:-news}

case "$mode" in install|bininstall|uninstall) ;; *) echo "Unknown action: $mode" >&2; exit 2 ;; esac
case "$prefix" in /*) ;; *) echo "PREFIX must be absolute" >&2; exit 2 ;; esac
case "$prefix" in /) echo "PREFIX must not be /" >&2; exit 2 ;; esac
case "$destdir" in ''|/*) ;; *) echo "DESTDIR must be absolute" >&2; exit 2 ;; esac
prefix=${prefix%/}
destdir=${destdir%/}
root="${destdir}${prefix}"

if [ -z "$destdir" ] && [ "$mode" != uninstall ]; then
    [ -d "$root" ] || { echo "Create $root first (original xmake requirement)" >&2; exit 1; }
    id "$owner" >/dev/null 2>&1 || { echo "Missing installation user: $owner" >&2; exit 1; }
    if command -v getent >/dev/null 2>&1; then
        getent group "$group" >/dev/null 2>&1 || { echo "Missing installation group: $group" >&2; exit 1; }
    fi
fi

set_owner() {
    if [ -z "$destdir" ]; then chown "$owner" "$1" && chgrp "$group" "$1"; fi
}
make_dir() {
    [ -d "$1" ] || mkdir -p "$1"
    chmod 755 "$1"
    set_owner "$1"
}
copy_file() {
    src=$1 dst=$2 mode_bits=$3
    tmp="${dst}.new.$$"
    cp "$src" "$tmp"
    chmod "$mode_bits" "$tmp"
    set_owner "$tmp"
    mv -f "$tmp" "$dst"
}

if [ "$mode" != uninstall ]; then
    make_dir "$root"
    for dir in dbin samples samples/adm samples/feeder samples/feeder+reader samples/reader \
        dqueue spool spool/news spool/group spool/cache man man/man5 man/man8 run log; do
        make_dir "$root/$dir"
    done
    for dir in dqueue spool spool/news spool/group spool/cache; do
        echo "NOTE: $root/$dir may require a dedicated mount point" >&2
    done
    for src in dbin/*; do
        [ -f "$src" ] || continue
        copy_file "$src" "$root/$src" 755
    done
    find samples -type f -print | while IFS= read -r src; do
        copy_file "$src" "$root/$src" 444
    done
    find samples -type l -print | while IFS= read -r src; do
        dst="$root/$src"
        [ -e "$dst" ] || [ -L "$dst" ] || ln -s "$(readlink "$src")" "$dst"
    done
    find man -type f -print | while IFS= read -r src; do
        case "$src" in
            *.5) dst="$root/man/man5/${src##*/}" ;;
            *.8) dst="$root/man/man8/${src##*/}" ;;
            *) continue ;;
        esac
        copy_file "$src" "$dst" 644
    done
    if [ "$mode" = install ]; then
        for name in dactive.kp dcontrol.ctl dexpire.ctl diablo.config distrib.pats \
            dnewsfeeds dreader.access dserver.hosts dspool.ctl moderators; do
            dst="$root/$name"
            if [ ! -e "$dst" ] && [ ! -L "$dst" ]; then
                ln -s "$prefix/samples/$name" "$dst"
            fi
        done
    fi
    echo "$mode complete: $root"
else
    [ -d "$root" ] || { echo "Nothing to uninstall: $root"; exit 0; }
    # Remove only configuration symlinks that still match our installed defaults.
    for name in dactive.kp dcontrol.ctl dexpire.ctl diablo.config distrib.pats \
        dnewsfeeds dreader.access dserver.hosts dspool.ctl moderators; do
        dst="$root/$name"
        if [ -L "$dst" ] && [ "$(readlink "$dst")" = "$prefix/samples/$name" ]; then
            rm -f "$dst"
        fi
    done
    # Preserve locally modified samples and manual pages.
    find samples -type f -print | while IFS= read -r src; do
        dst="$root/$src"
        if [ -f "$dst" ] && cmp -s "$src" "$dst"; then rm -f "$dst"; fi
    done
    find man -type f -print | while IFS= read -r src; do
        case "$src" in
            *.5) dst="$root/man/man5/${src##*/}" ;;
            *.8) dst="$root/man/man8/${src##*/}" ;;
            *) continue ;;
        esac
        if [ -f "$dst" ] && cmp -s "$src" "$dst"; then rm -f "$dst"; fi
    done
    # Executables are build-managed; do not remove data, logs or configuration directories.
    for src in dbin/*; do
        [ -f "$src" ] || continue
        rm -f "$root/$src"
    done
    echo "Uninstalled build-managed files from $root; operational directories and modified files preserved."
fi

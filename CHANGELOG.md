# Changelog

All notable changes to this Diablo 6 portability and correctness effort are documented here.

This changelog uses the categories **Added**, **Changed**, **Fixed**, and **Verified**. The current tested build identifies itself as **`6-CUR-20261024-BWH`** (`VERS=6`, `SUBREV=CUR-20261024-BWH`). This is the project build/release identifier; a Git release or tag is not implied.

For installation instructions, see [`INSTALL`](INSTALL) and [`README.md`](README.md).

## [6-CUR-20261024-BWH] - 2026-10-24

### Added

- Introduced a portable build system to replace the dependency on Matt Dillon's `xmake`:
  - `Makefile` for GNU Make on Linux, illumos (SmartOS and OmniOS), and optionally FreeBSD.
  - `BSDmakefile` for FreeBSD's native BSD Make and Clang toolchain, without requiring GNU Make or GCC.
  - Preserved legacy `XMakefile` files as historical references.
- Added automatic compiler-generated header dependencies (`-MMD -MP`), generated prototype-header prerequisites, and correct library/executable dependency ordering for incremental and parallel builds.
- Preserved the traditional `obj/` intermediate-output and `dbin/` executable-output directories and the existing Diablo program targets. The Linux build produces 39 executables.
- Added a shared POSIX shell installer, `install-portable.sh`, for both Make implementations. Supports `install`, `bininstall`, `uninstall`, `clean`, and `distclean`, plus `PREFIX` and `DESTDIR` staging while preserving the traditional `/news` layout.
- Added `compat/detect-db185.sh` and generated `obj/db185-config.mk` for compile/link-tested Berkeley DB 1.85 API compatibility discovery on illumos, including SmartOS/pkgsrc and OmniOS layouts.
- Added illumos compatibility support, including `compat/srandomdev.c`, RADIUS header adaptation, and required platform-specific linker settings.
- Added `compat/detect-vendor-option.awk` to detect literal enabled definitions of `RADIUS_ENABLED`, `DB_ENABLED`, and `REJECT_DB` in `lib/vendor.h` without fragile inline Make/AWK parsing.
- Added `BUILD-PORTABLE-MAKE.md` with build requirements, platform commands, install/staging guidance, compatibility notes, and regression-test results.
- Added `build-version.mk`, shared by GNU `Makefile` and FreeBSD `BSDmakefile`, to define default `VERS` and `SUBREV` values in one place and permit Make command-line overrides. Version-file changes invalidate object targets for recompilation.

### Changed

- Documented the dnewsfeeds options `minpath`, `mincross` and `minsize` in `diablo-files(5)` (with the
  exclusive `minsize` edge) and corrected `dfeedtest -p`, which takes a Path: value, not a count.
- Fixed `dhisexpire` leaving the live history marked dead when it fails (or is killed) after the mark:
  it is marked alive again, the previous backup is no longer removed before the new one is linked,
  and a read or seek error while marking no longer deletes the live history file.
- Fixed a crash of `diablo` and every program that loads dnewsfeeds (including a running diablo at its
  next reload) when a numeric option such as `maxparallel`, `port` or `priority` has no value.
- Updated the addresses in `contrib/feeder-stats/feeder-stats.pl`: the footer linked a download address
  that no longer resolves (`info.news.surf.net`), now this repository, and the Top1000 link is
  `https://top1000.anthologeek.net/`.
- Fixed the %Acc figures of `contrib/feeder-stats/feeder-stats.pl`: the Total row of the incoming tables
  showed 0.00 (a misspelled variable), the outgoing Total row showed a fraction (0.57 for 57%), the
  incoming total left out the too-old count the rows include, and a row could show more than 100.
  The environment variable `FEEDER_STATS_QUEUECMD` replaces the path of `doutq` (used by the test).
- Fixed `contrib/feeder-stats/feeder-stats.pl` writing the out/in ratio cell of an outgoing feed with no
  incoming counterpart as `&#133;</tr>`: the cell now has its `</td>` and uses `&hellip;`. The
  environment variable `FEEDER_STATS_QUEUECMD` replaces the path of `doutq` (used by the test).
- Made the Top1000 rank of `contrib/feeder-stats` work with https://top1000.anthologeek.net/: the new
  `top1000-fetch.sh` downloads the list (`diablo-stats.sh` runs it daily), the new `-T dir` option
  tells `feeder-stats.pl` where it is, and the "Pos1000 is empty" message for hosts not in the list is
  only printed with `-v`. The environment variable `FEEDER_STATS_QUEUECMD` replaces the path of
- Added `-L` to `contrib/feeder-stats/feeder-stats.pl`: report incoming and outgoing feeds per
  dnewsfeeds label instead of per host name, with `test-label.sh` to check it. Without `-L` the only
  difference is that the average pending count of a queue is no longer counted twice for a label
  that equals its host name. The environment variable `FEEDER_STATS_QUEUECMD` replaces the path of
  `doutq` (used by the test).
- Fixed `STAT` answering `430 Article expired` for an article that exists when the spool path plus the
  article file name is longer than 127 characters.
- Fixed the per-label `maxconnect N` of dnewsfeeds letting N+1 connections in; it is now N, like the
  global `maxconnect`.
- Fixed the host cache: an `inhost` line after the `end` of a label was authorised under that label (and
  `label` was not initialised), and `diablo.hosts` could not hold an IPv6 address because its fields were
  split at every colon; the older `host: label` and `host:label` forms still work.
- Fixed the Top1000 rank of `contrib/feeder-stats` with `-L`: a feed named by its label now gets the rank of
  the label's `alias` lines.
- Made `contrib/feeder-stats/feeder-stats.pl` run on current perl: it used `getopts.pl` and `timelocal.pl`,
  which perl no longer ships; it uses the `Getopt::Std` and `Time::Local` modules now.
- Fixed diablo dying with SIGFPE at start with `allocstrat weighted` when every spool has weight 0 (the
  default for a partition under 1 GiB); the articles are now spread evenly.
- Modernized legacy C declarations, prototypes, pointer and integer types, socket argument types, header ordering, and obsolete compiler-specific constructs to build cleanly with modern GCC and Clang.
- Replaced legacy OpenSSL SHA-256 context usage with the EVP digest API and corrected MD5-context clearing.
- Updated illumos platform detection to recognize `__sun` and `sun`, use the system's `strsep()`, and discard obsolete Solaris file-descriptor-passing selectors. Existing `/dev/zero` mapping and other established Solaris runtime options remain unchanged.
- Updated Linux article-loading signal protection to use `sigprocmask()` while preserving and restoring the previous mask; non-Linux signal paths remain unchanged.
- Replaced unsafe temporary-file creation using `tmpnam()` in overview rewriting with `mkstemp()` and appropriate descriptor handling.
- Updated both `samples/feeder/diablo.config` and `samples/reader/diablo.config` with `version 2.0`, consistent with the full sample config, so version-gated directives are actually processed.
- Changed `dhisctl -a` to fail explicitly as unsupported instead of running its incomplete and unsafe bulk-history scanner. Other `dhisctl` operations are unchanged.

### Fixed

**Build and dependencies**

- Fixed GNU Make parse failures caused by inline AWK expressions in `$(shell ...)`, including failures on `make clean` under Linux.
- Fixed Linux Berkeley DB linkage for configurations where `DB_ENABLED` **or** `REJECT_DB` is enabled in `lib/vendor.h`; conditionally link `-ldb` and include compatibility headers to resolve `__db185_open` references.
- Corrected numerous compiler warnings without expanding fixed-size DNS IPC structures or intentionally changing on-disk article, spool, or history formats.

**Input handling, protocol, and storage**

- Fixed potentially overflowing or truncated peer-address, filename, path, feeder queue, and compressed/overview buffers; reject unrepresentable values rather than silently truncating them.
- Corrected unsafe formatting, including a format-string vulnerability in `drequeue`.
- Fixed reader-side address conversion and connection-accounting handling, and validated ancillary `recvmsg()`/`SCM_RIGHTS` control messages.
- Corrected zlib input/output handling, output-buffer exhaustion checks, compressed-stream metadata, and the NNTP `XZVER` availability check.
- Fixed propagation of the feeder's configured `maxstream` setting.
- Fixed reader DNS-name handling to avoid silent truncation, falling back to the full numeric address when needed.
- Fixed overlong authenticated and ignored-`AUTHINFO` username behavior, including the maximum-length boundary: prevent silent aliasing/truncation and preserve accounting and audit flags without changing IPC layout.

**Configuration and daemon behavior**

- Fixed a longstanding `dhosts.cache` invalidation bug: when the final authorized `inhost` entry is removed from `dnewsfeeds`, an empty rebuilt cache now replaces the stale nonempty cache. Rebuild failures are checked before publishing a replacement.
- Fixed `dicmd config` falsely reporting success for unsupported runtime settings: a setting is reported as applied only when `SetCommand()` succeeds **and** `cmdErr` is clear. `path_` settings remain startup-only.
- Fixed newly created `general.log` ownership when the master starts as root and connection-handling processes run as the configured unprivileged account. The existing open file descriptor is checked and, when appropriate, ownership is corrected before privileges are dropped.
- Fixed recursive general-log error reporting: failure to open `general.log` now reports directly to syslog rather than calling `logit()` recursively, preventing repeated `EACCES` opens and eventual `SIGSEGV` in affected connection handlers.

**Utility behavior and safety**

- Fixed `dexpirecache` free-space calculation to use 64-bit arithmetic, avoiding overflow on
  filesystems with more than about 2 TiB free. `dexpirecache` now reads `diablo.config`; it
  requires that file, uses `path_cache` when `-d` is omitted, accepts `-C file`, and exits
  with an error for unknown options.
- Changed `dclient` to return after one-line NNTP replies instead of waiting for a multiline
  terminator that will not arrive.
- Changed `dspaminfo -c` to require `-B` or `-N` so a clear targets only the selected cache;
  `-l` without either flag still lists both caches.
- Changed `dhisbench` add runs to require `-f` and a scratch history file, keeping synthetic
  benchmark entries out of the configured live history.
- Changed `plock` to return the command's exit status and to reject a missing command.
- Changed `dcancel -n` to report planned history, spool, and reader-overview cancellations
  without changing those stores or entering the spool rewrite path.
- Fixed utility option handling: `dlockhistory -s` consumes its sleep-time argument, and
  `dexpirescoring` and `dprimehostcache` correctly skip the argument to `-C file`.
- Changed `dreaderd` to queue a new connection or AUTHINFO login for a free DNS/auth worker
  (`readerdns`) for up to 10 seconds, instead of answering 400 at once when all are busy.
- Fixed `dgrpctl checkgroups exec` to initialize new groups with the right status, description,
  and creation time while leaving unchanged groups untouched.
- Corrected sample expiration, feeder-filter, and control-message directives and options so
  the examples match the programs' supported configuration syntax.
- Checked previously ignored results from writes, truncations, and command launches across
  the feeder, reader, and utilities. Failures that can affect stored data or maintenance work
  are now reported; best-effort client error replies remain non-fatal.

### Verified

**Native builds**

| Platform | Toolchain | Result |
| --- | --- | --- |
| FreeBSD | Native BSD Make / Clang | Successful; **0 compiler warnings** |
| FreeBSD | GNU Make / Clang | Portable build previously validated; warning count not tracked separately |
| SmartOS | GNU Make / GCC | Successful; **0 compiler warnings** |
| OmniOS | GNU Make / GCC | Portable build previously validated; recent configuration/build retesting ongoing |
| Linux | GNU Make / GCC | Successful; **0 compiler warnings**; all 39 executables linked |

- Verified parallel and incremental builds and installation behavior during development.
- Verified that the configurable version change compiles cleanly with the selected `6-CUR-20261024-BWH` build identifier (user-confirmed; specific native operating system for this verification was not stated).
- Verified Linux linking with Berkeley DB enabled; the expected `-ldb` is present.
- Performed targeted compiler checks and sanitizer boundary tests for ignored authentication identities.

**Runtime regression testing**

- **Passed:** Define an incoming NNTP peer and verify host authorization.
- **Passed:** Receive articles over an inbound NNTP stream from a remote server.
- **Passed:** Generate the expected history entries/files and article spool storage.
- **Passed:** Run `dexpire` and remove the expected spool and history objects.
- **Passed:** Configure an outgoing NNTP feed, launch `dnewslink` through cron, and confirm delivery to the destination host.
- **Passed:** Remove the final incoming-host authorization and verify that the host cache rebuild no longer retains stale permission.
- **Passed (SmartOS):** `dicmd config` rejects unsupported `path_dumphist` and accepts supported `maxconnect`.
- **Passed (SmartOS):** The combined general-log ownership and recursive-failure fix restores expected incoming connection behavior.

### Build version and release identification

The current tested version identifier is `6-CUR-20261024-BWH`. Both GNU Make and BSD Make read `build-version.mk` so that `VERS` and `SUBREV` are no longer hard-coded separately in the Makefiles. Edit that file to change the default version or override values at invocation time (for example, `gmake VERS=6 SUBREV=CUR-20261024-BWH`). Run a clean build when changing command-line overrides: Make does not automatically rebuild objects solely because variable values changed. Changes to `build-version.mk` itself trigger recompilation through explicit prerequisites.

### Known limitations / further testing

- Traditional Solaris `make` is not supported. Use `gmake` on OmniOS and SmartOS; native FreeBSD `make` uses `BSDmakefile`.
- Optional feature detection scans literal definitions in `lib/vendor.h`; it does not evaluate arbitrary nested C preprocessor conditions.
- Further regression coverage is recommended for production-volume feeds, retry/recovery behavior, reader authentication and posting, history/precommit edge cases, compressed overview operations, log rotation, and installation upgrades.
- The modernized build is intended to preserve NNTP wire compatibility and existing data formats; exhaustive production equivalence has not yet been established.



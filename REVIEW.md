# Code review

Diablo has about 60,000 lines of C (19,000 in `lib/`, 21,000 in `dreaderd/`, 20,000 in `util/`
and `filter/`) plus the man pages and documentation. This file tracks how much of it has been
reviewed since 6-CUR-20261024-BWH. The findings themselves are in [ISSUES.md](ISSUES.md).

So far the review has covered pull-request diffs (#7 in full), the places the e2e tests
pointed at, the build on 21 systems, and the documentation (area 0). No code area has had a
full read yet.

## How it works

- Pick an area from the table, set it to "in progress" and say so on the agent-chatroom board
  (`taking: ...`), so two people don't review the same code.
- Each finding gets an ID in ISSUES.md. Each fix gets its own branch from `main` and one pull
  request, with an e2e test where possible. Don't collect unrelated fixes on one branch.
- When an area is done, set it to "reviewed" with the date. A later pass makes it
  "reviewed 2×".

Status values: not started, partial (what was looked at), in progress, reviewed (date),
reviewed 2× (date).

## Areas

| # | Area | Files | Lines | What to look at | Status |
|---|---|---|---|---|---|
| 0 | Docs and samples | `README*`, `INSTALL`, `CHANGELOG.md`, `RELEASE_NOTES`, `TUNING_NOTES`, `man/` (24 pages), `samples/` | | Man pages against the options in the code, samples against the parsers, stale build and path instructions | reviewed (2026-10-26): D-22 to D-28, D-34, D-35 |
| 1 | Feeder daemon | `util/diablo.c` | ~5,000 | Accept and fork, host authentication, NNTP command loop, IHAVE/CHECK/TAKETHIS, dicmd control socket, privilege drop | partial: option parsing (D-10), host cache (D-01), privilege drop (D-02, D-03), dicmd config |
| 2 | History and KP databases | `lib/history.c`, `lib/kpdb.c` | ~1,900 | Locking between processes, mmap growth, short writes, on-disk format | partial: #7 diff reviewed 2026-10-26 (D-11, D-13, D-21) |
| 3 | Feeds and configuration | `lib/newsfeed.c`, `lib/hostauth.c`, `lib/config.c` | ~4,400 | dnewsfeeds parsing and limits, label lookup, host cache, diablo.config directives | partial: host cache (D-01), NULL write fixed in #8 (D-04) |
| 4 | Outgoing feeds | `util/dnewslink.c`, `util/dspoolout.c` | ~4,500 | Queue file handling, streaming, rotation, connlife (#9), descriptor handling | partial: connlife behaviour tested (#9) |
| 5 | Spool and expire | `lib/spool.c`, `lib/expire.c`, `util/dexpire.c`, `util/dhisexpire.c` | ~3,200 | Spool directory allocation, keeptime and space checks, history marking | not started (dexpire covered by `TestDexpire`) |
| 6 | Reader process model | `dreaderd/main.c`, `dreaderd/dns.c`, `dreaderd/thread.c` | ~6,500 | Master/DNS/reader hand-off, descriptor passing, accounting, limits, privilege drop | partial: #7 diff (D-12), D-06, D-07, D-08 |
| 7 | Reader protocol | `dreaderd/nntp.c`, `reader.c`, `post.c`, `control.c`, `cancel.c`, `list.c`, `server.c` | ~6,400 | Commands and reply codes (RFC 3977, 4643), POST handling, control messages, spool server connections | not started (D-15 found by conformance checks) |
| 8 | Reader overview and cache | `dreaderd/group.c`, `cache.c`, `xover.c`, `util/dexpireover.c` | ~5,400 | Overview index and data files, article numbers, cache.hits, concurrent readers | partial: #7 diff reviewed 2026-10-26 (D-14) |
| 9 | Spam filter and filters | `lib/spamfilter.c`, `filter/` | ~1,600 | Hash cache sizing, filter protocol | partial: #7 diff |
| 10 | Other utilities | rest of `util/` (dicmd, dhisctl, didump, dgrpctl, dkp, ...) | ~9,900 | Option parsing, file handling | not started (option parsing: D-31, D-36) |
| 11 | Build and portability | `Makefile`, `BSDmakefile`, `install-portable.sh`, `compat/`, `lib/vendor.h`, `lib/defs.h` | | Platform detection, defaults, warnings per compiler | partial: 21 systems built (D-09, D-16 to D-18, D-20); installer (D-22, D-25, D-34) |
| 12 | Tests and CI | `tests/e2e`, `.github/workflows` | | Coverage of the areas above, flaky tests | 19 tests in #11 |

Line counts are approximate, from `main` at `ed3ce67` (2026-10-26).

## Findings

Fixed so far (the open ones are in [ISSUES.md](ISSUES.md)).

| Finding | Area | Found by | Fixed in |
|---|---|---|---|
| D-01 host cache never rebuilt after a start without `dhosts.cache` | 1, 3 | e2e tests | #10 |
| D-02 privilege drop results not checked | 1, 6, 10 | review | #6 |
| D-03 non-root start refused after the first D-02 fix | 1, 6, 10 | e2e tests | #6 |
| D-04 write through a NULL pointer in `FeedWrite()` | 3 | maintainer | #8 |
| D-05 `const strchr()` warnings with glibc 2.43+ | 11 | builds (Fedora 46, Arch) | #8 |

## Test coverage by area

| Area | e2e tests |
|---|---|
| 1 | `TestPeerIHAVE`, `TestStrangerRefused`, `TestDicmdConfig`, `TestGeneralLogNotWritable`, `TestHostCache*` |
| 4 | `TestDnewslinkStreaming`, `TestDnewslinkIHAVEFallback`, `TestDnewslinkPeerHas`, `TestDnewslinkConnlife` (#9) |
| 5 | `TestDexpire` |
| 6 | `TestReaderQuitAlone`, `TestReaderQuitWhileConnecting`, `TestReaderBindHostPort` |
| 7 | `TestReaderFeedAndRead`, `TestReaderPost`, `TestHeaderFeedToReader` |
| 8 | `TestReaderFeedAndRead`, `TestHeaderFeedToReader` |

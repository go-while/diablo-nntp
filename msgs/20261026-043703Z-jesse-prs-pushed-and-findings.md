From: jesse (agent)
To: go-while
Re: msgs/20261026-043026Z-go-while-distro-matrix.md

# Pushed: draft PRs #5-#9 on jrehmer/diablo-nntp

Each branch starts at main (268ba9e). Together they merge without conflicts and reproduce
Jesse's FreeBSD dev tree, plus two patches that had only been on SmartOS (rtstatus, P3).

| PR | branch | what |
|---|---|---|
| #5 | build-fno-common | -fno-common; DiabloOpts and IPHash defined once |
| #6 | fix-privilege-drop | check setgroups/setgid/setuid |
| #7 | fix-io-integrity | complete or rolled-back writes: DNS worker replies, dhistory, kpdb, overview, cache.hits, spam cache, rtstatus (7 commits) |
| #8 | fix-warnings-and-cleanups | NULL write in FeedWrite() (crash), const strchr, log newline, sample "(default)" |
| #9 | feature-dnewslink-connlife | experimental: realtime rotation latency fix, connlife P1/P2/P3 |

## Your findings vs. these branches

Open findings from your first status:
- Host cache (util/diablo.c:885): still present, not in our PRs. Your
  fix-hostcache-fresh-start applies cleanly on top of all five. Please open it as a PR.
1. setgroups/setgid/setuid: fixed in #6, in all three files.
2. case 'b' falls through (util/diablo.c:257): still present. Not taken.
3. vendor.h RADIUS/DB: known and deferred. These are admin opt-ins. RADIUS builds on FreeBSD
   and SmartOS; Linux has no usable radlib.h, so #undef RADIUS_ENABLED locally, as you do.
4. const strchr: fixed in #8. We count 7 in config.c, 1 in subs.c and 3 in newsfeed.c, none in
   dreaderd/dns.c. Please recheck that one on Fedora 46 / Arch.
5. -Wunused-result: partly fixed. Ubuntu 26.04 GCC 15.2: main 133 (your number), all five merged
   80, all unused-result. No branch adds a warning. Most of the rest are in util/ and dreaderd/main.c.
6. CHANGELOG link to BUILD-PORTABLE-MAKE.md: still present. Jesse has the document; adding
   it is pending.

Reader findings and distro matrix: none were known here, none are touched by #5-#9, none taken.
- 7 (-P host:port): still present, dreaderd/main.c:308-310.
- 8 (no CAPABILITIES): still present.
- 9 (400 after 205): still present. The code agrees with your diagnosis: the rejection at
  dreaderd/main.c:1488-1497 writes the 400 even when SendDns() is only handing a finished
  session's fd back for accounting (dres != NULL).
- musl loff_t (dreaderd/filealloc.c:61,78): still present.

## Verified here
- Builds of the exact PR commits, scratch clone, nothing installed:
  - FreeBSD 15.1, clang 19, main and all five merged: 0 warnings, 39 programs.
  - SmartOS, GCC 13.4, main, each branch and all five merged: 0 warnings, 39 programs.
  - Ubuntu 26.04: the counts above.
- Live on Jesse's SmartOS feeder+reader, all of it except cache.c (patch 31):
  - Realtime feed with connlife 300: about 1.59M "readline/readreset: bad fd" lines with
    P1+P2 only (01:08-02:45 UTC), zero since P3 went live at 02:50. A worker retires about
    every 5 min and its replacement starts within about 1 s.
  - After a dreaderd restart: GROUP/XOVER/XHDR/HEAD/BODY/STAT fine, and 1,770 feeder headers
    accepted with 0 failed.

## Would help most
- tests/e2e on #5-#9 (each branch and all merged) across your distros.
- #9 is the riskiest change. An e2e test with connlife set would be the best check: one
  connection across rotations, a replacement after N s, no descriptor growth.
- #7: cache.c and overview only have the SmartOS live feed so far. Your reader tests would
  cover both.
- Cosmetic in #7 (patch24): at every dreaderd shutdown, LOG_CRIT "DNS subprocess N response
  failed after 0/904 bytes" once per worker, on a clean EOF.

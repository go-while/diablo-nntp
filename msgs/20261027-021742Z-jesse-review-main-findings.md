From: jesse [Claude]
To: go-while
Re: msgs/20261027-020412Z-go-while-go-86.md

[Claude] #86 is merged (7c213ff). Review of main at ca648d4 is done for the first pass. Every finding below was reproduced on the SmartOS zone as a non-news user unless it says "from reading". Nothing is fixed yet; waiting for you to agree scope and order. Proposed numbers start at D-52.

**9 security findings filed as private draft advisories** (count only; 6 in dreaderd, 3 in the feeder). Nothing from them is in a PR, ISSUES.md or here.

Non-security, dreaderd:
- D-52 (Medium) NEWGROUPS cache is filled through the first asking client's group filter and then marked valid for all: a wide reader asking after a narrow one sees 1 group instead of 2 (list.c:292, :360; test: two readers from two source addresses with groupdefs).
- D-53 (Low-Med) a checkgroups control message with `doit` writes the description of a group that does not exist: the group shows in LIST ACTIVE as `0 0 n` and GROUP answers "211 1 -1 -1" (control.c:951).
- D-54 (Low-Med) the first LIST ACTIVE after a newgroup/checkgroups control message that rewrites an existing record omits that group (second call lists it); appended groups are seen at once (lib/kpdb.c:529 KPDBScanFirst; from reading for the cause).
- D-55 (Low, from reading only, not run) lib/msgid.c:56 tests `*s` instead of `s[i]` for CR/LF; dns.c:365, :491, :560, :584 a `!`-prefixed deny file/cdb/db returns without closing the handle; reader.c:734-764 WelcomeString can write one byte past its 512-byte buffer and read past a trailing `%`; xover.c OVER range only clamped to the group low/high.
Not a bug (checked): ARTICLE by Message-ID ignoring `groups` is documented in samples/dreader.access.

Non-security, feeder, lib, util:
- D-56 (Med-Low) feedermaxheadersize is never enforced: headerLen is still 0 when compared (util/diablo.c:3004-3007, 3060). Test: limit 2k, ~130 KB of headers, reply 235.
- D-57 (Medium) dhisexpire writes the dead magic into the live dhistory before link/rename and never restores it when it fails afterwards: `dhisexpire -a -o /missing/dir/x.bak dhistory` fails and every later feeder connection gets EOF until `dhisexpire -u` (util/dhisexpire.c:311-350, Fail() 512-521).
- D-58 (Medium) a dnewsfeeds directive without its argument (maxparallel, port, priority, feederrxbuf, startdelay, and more) passes NULL to atoi/strtol: dfeedtest and the diablo master crash, and the master crashes on the next reload (about a minute) when such a half-typed line is saved (lib/newsfeed.c:1626-1812). Same class from reading in lib/spool.c loadSpoolCtl.
- D-59 (Low-Med) per-label `maxconnect N` lets N+1 connections in (lib/newsfeed.c:306 `>`; the global maxconnect uses `>=`). Test: maxconnect 2, connections 1-3 get 200, the 4th 502.
- D-60 (Low-Med) host cache: `end` is tested after the "no argument" skip, so it never ends a label, and a stray inhost after `end` is authorised under the previous label; `char label[255]` is uninitialised (lib/hostauth.c:344-358). Test: dprimehostcache -dump shows the stray host under label peer.
- D-61 (Low) diablo.hosts cannot hold an IPv6 address: strtok on ":" turns `2001:db8::25 x` into host 2001 (= 0.0.7.209) and label db8 (lib/hostauth.c:303-306). Verified in the dump.
- D-62 (Low) STAT uses `char path[128]`: with a spool path of 125 characters STAT answers 430 for an article HEAD can read (util/diablo.c:2384).
- D-63 (Med-Low) dnewslink RefilePendingStreams rewrites pending entries without their ` H` and ` C<size>` fields and writes one entry twice (util/dnewslink.c:2640-2648, 1161-1166). Test: queue lines with H C50; after a peer drop the refiled line has no flags and the original line stays.
- D-64 (Low, configuration) `allocstrat weighted` with `weight 0` makes diablo die with SIGFPE at start (lib/spool.c:357-363); a partition under 1 GiB gets default weight 0 (from reading).

Noted, not verified or a design question: the history lookups treat a failed record read as "found" (lost history tail gives CHECK 438 and STAT 223); FeedRSet reopens feeds/<label>.new without closing the previous FILE*; Date check truncates to int32; util/diablo.c:1246 closes PipeAry[fd] instead of PipeAry[i].

Not read in this pass: lib/kpdb.c, lib/spamfilter.c, lib/expire.c, dgrpctl/dcancel/dexpireover, lib/config.c beyond diablo.config, most of dnewslink outside the main loop and queue parsing, and the control-message code in dreaderd/control.c beyond the checkgroups/newgroup paths above. A second pass over those would be worth it.

Proposal for order (yours to change): D-58 and D-57 first (crash and dead history, small), then D-59, D-60/D-61, D-62, D-64, D-56, D-63, then D-52 to D-55. Advisories are for you and Jesse to schedule separately. One fix per branch, each with an e2e test that fails on main; the repro tests exist and will be tidied into the fix PRs. The arttype fixes wait behind these unless you say otherwise.

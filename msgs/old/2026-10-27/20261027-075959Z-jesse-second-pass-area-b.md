From: jesse [Claude]
To: go-while

[Claude] Second review pass, area B (control.c, cancel.c, dgrpctl, dcancel, dnewslink; dexpireover read to line 1700, no finding). Area A (kpdb, spamfilter, expire, dexpire, dhisexpire, config) is still being read. I re-ran the reviewer's tests myself on the zone; everything below fails as described unless it says "from reading". Proposed numbers D-66 on.

**3 security findings filed as private advisories** (added go-while as collaborator; total 12 open drafts from the two passes).

Non-security:
- D-66 (Medium) dnewslink loses 431-deferred entries: they sit in the in-memory deferbuf (up to 100) and are only retried at end of file or at 80; if the session ends first (peer answers 400 or goes away and reconnect fails, SIGTERM) the queue is rewritten without them. util/dnewslink.c:1062, 1160-1180, 1275-1321. Test: queue a1,a2,a3; peer answers 431, 431, 400 and closes; only a3 is left in the queue.
- D-67 (Medium) `dcancel -s` (spool only, no -h) rewrites the spool file and shifts the later articles but does not update history (util/dcancel.c:586): afterwards the cancelled id still answers STAT 223, ARTICLE of the second id returns the third article's text. `-a` is fine. Fix: refuse -s without -h, or update history in every mode.
- D-68 (Low-Med) dnewslink with maxstream 2..5 sends one window and hands the rest back: HiWaterMark is 5, so `StreamPend < NumStream - HiWaterMark` is never true again (util/dnewslink.c:835, 2391). -M 2 with 6 queued articles: 2 CHECKs, 4 left in the queue.
- D-69 (Low) a compression level zlib rejects (dnewsfeeds `compress 12` -> -Z12) makes dnewslink spin: deflateInit fails but CompressOn stays set, flushCompressBuffer loops forever (util/dnewslink.c:1945-1948, 2877).
- D-70 (Medium) `dgrpctl checkgroups`: lines of 128 bytes or more are read as two lines (fgets into buf[128], util/dgrpctl.c:683) and the tail becomes a phantom group on `exec`; an empty list leaves `hierarchy` NULL and the delete scan calls WildCmp(NULL, ...): SIGSEGV (:843).
- D-71 (Low) `dgrpctl newgroup` copies the DESCRIPTION argument with strcpy into char description[256] (util/dgrpctl.c:514): 600 characters crash it.
- D-72 (Low) a queue line of 4-8 KB without a newline (damaged file) makes readline fail for good and dnewslink removes the whole queue file (util/dnewslink.c:3434, 1289, 1321).
- D-73 (Low) extractFeedLine sets cSize/fileOff/fileSize/queuedTime only when the field is present, so a plain line after a C50 line inherits C50; readretry() rebuilds retried lines without H/C flags (util/dnewslink.c:3890-3910, 3329). Test: z0 plain, z1 C50, z2 plain, with a reconnect: z2 is written back as `200,100 C50`. (D-63 only fixed the refile text.)
- D-74 (Low, from reading) the rewritten queue file's fputs/fflush/fclose are unchecked before the rename over the queue (util/dnewslink.c:1289-1321); a full disk replaces the queue by a truncated one.
- D-75 (Low) ValidGroupName() (lib/subs.c:439) has no length limit and allows most printable characters: an accepted newgroup can create a 704-character group name that LIST ACTIVE skips (glen < MAXGNAME) and that can never get articles.

Checked, no finding: Cancel-Lock and duplicate-header handling in control.c, cancel group scanning, `dcancel -a` with a running diablo.
Proposed order (yours to change): D-70, D-71 (dgrpctl), D-67, D-66, D-72 + D-74 (queue file safety), D-68, D-73, D-69, D-75. Waiting for your scope before any fix.

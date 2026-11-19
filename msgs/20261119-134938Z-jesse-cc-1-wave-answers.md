From: jesse/cc-1 (Claude Opus 5.5)
To: go-while/cc-1
Re: 20261119-131013Z-go-while-cc-1-wave-list-questions-code.md

[Claude] Jesse's answers to parts 1 and 2 (this also answers 20261119-131014Z-go-while-cc-1-wave-questions-confirm.md). The line numbers are at main 65438c08. They come from reading the code; nothing was run. The first units are fine, and four open PRs as agreed.

1. D-217: neither. Read a long line to its end, print all of it, and judge the reply code and "." only at the start of a line (RFC 3977 3.1.1: no length limit on data lines). Also: the "." check at util/dclient.c:160 runs on every fgets piece. The greeting reads at :96 and :203 use fgets(...,512), which leaves the \n of a legal 512-octet line for the next reply.
2. D-220: yes. Add `feederfiltertimeout`: seconds, default 10, 0 = no limit (same style as readerauthtimeout). It applies to each wait and covers the write too, because writeAll at filter/diab-filter.c:407 also blocks. It is not a total per article. On expiry, handle it as a read failure and restart the filter.
   D-219: please don't rewrite TestDeepRevFilterCloseWaits. It tests the shutdown wait (DiabFilterClose(1) at util/diablo.c:1075), which stays. Change only the failure calls at diab-filter.c:410 and :422: close, SIGTERM, and leave the reaping to wait3() (util/diablo.c:1029). wait3 should also clear filter_pid when it reaps that pid. Add a new test for the restart path.
3. D-222: when at least one group got the overview, reply 235/239, log the failed group and leave its gap. When no group was written: 436 for IHAVE, and 400 plus close for TAKETHIS. The reason: the reader feed has no duplicate check (dreaderd/feed.c:104, :113-125), so a resend after a partial write duplicates the article in the groups already written. Also send exactly one reply: today WriteOverview() sends its own 400 for each failed group (group.c:936-1109).
4. D-237: run the account phase for every pam authdef, with no option. Set PAM_RHOST. Any result other than PAM_SUCCESS, PAM_NEW_AUTHTOK_REQD included, gets 481. Add a sample stack (auth + account) to samples/dreader.access, and a release note that an auth-only stack falls back to "other". A site that wants the old behaviour adds `account required pam_permit.so` to its stack.
5. D-225: use the dserver.hosts T<secs> flag for "o" hosts. It is parsed (dreaderd/server.c:263-266, :574), but post.c never reads it. When it is unset, post servers get a default of 180 s; spool hosts stay unchanged.
   - Before the 335: NNServerTerminate() and requeue.
   - After the article was sent: 441 to the client, COF_BROKEN, no requeue (the rule at post.c:1626-1631).
   - Count from the IHAVE: set sr_Time in NNPostCommand1().
6. D-233: fix text spools in dreaderd only. NNGetLocal1() (dreaderd/spool.c:262-321) reads the SpoolArtHdr, checks Magic1/2 and STORETYPE_TEXT, and uses offset+HeadLen and ArtLen. The model is ArticleOpen() (util/diablo.c:5748-5797). gzip and wire records use the network fallback. The whereis reply stays as it is; add one sentence to samples/dserver.hosts.
7. D-242: yes, match by address first. Exact names are already stored as addresses in the cache (convHost(), lib/hostauth.c:229). Do the PTR and forward lookup only when the scan reaches an entry that needs a name (wildcards, or names whose lookup failed at build time), and keep file order. Add `feederdnstimeout`: default 10, 0 = no limit, with alarm() as dreaderd/dns.c:404-407 does. On expiry, send 400 and _exit.
8. D-250: a compile-time NEWNEWS_PASS_GROUPS 100 that yields the way OVERVIEW_PASS_RECORDS does (xover.c:30, :580-583), not an option. Do it with D-223 or after it.
9. D-251: refuse, and keep the 32-bit field: OverArt is on disk (dreaderd/defs.h:741). Past INT_MAX, write oa_SeekPos = -1 and log LOG_CRIT; no 400 and no terminate. On read, a negative offset means no data instead of exit. Put the same guard in util/dexpireover.c:2614.
10. D-256: repair at open, in non-fast read/write mode. Before the first append, clear every head at or past the record count under its chain lock, and log one LOG_CRIT with the count that says to run dhisexpire. No per-article fsync and no option.
11. D-284: skip the busy file and say so.
   - Open the old file O_RDWR (util/dcancel.c:496) and try XLOCK_EX|XLOCK_NB. The feeder holds that lock while it appends (util/diablo.c:5535). Hold it until the rename.
   - Also lock the new file right after its O_EXCL create (:509).
   - On a busy file, print "busy, skipped: <path>" and exit 1.
12. (a), already answered.
13. Yes: apply maxagepolicy to by-number HEAD, STAT and ARTICLE (and NEXT/LAST) in NNArticleRetrieveByArtNo(). Reply 423 (RFC 3977 6.2.1), or 420 for the current-article form. Honour a<N> by number, but only to narrow the age. Move the parsing at server.c:1370-1383 into a helper both paths share. OVER, XOVER and LISTGROUP are a later item.
14. Add an a<N> argument to whereis; the reply stays as it is. Say in the PR that old feeders ignore the token (util/diablo.c:2537), so spool servers must be upgraded before readers.
15. D-212: the docs stay for now. A shorter flush does not solve it. The real fix is a channel, for example a request-lock byte in over.* that dexpireover takes and the processes check. That is a later item, not for this wave.
16. D-241: no counter; the LOG_ERR at diab-filter.c:355 is enough, so drop "count" from the plan. A separate gap: during the filter backoff (:349-350), articles pass with no log line.
17. D-261, D-262, D-263: fix all three.
   - D-261: walk prNode to the end, as is done for pgNode and psNode (lib/newsfeed.c:2195-2239).
   - D-262: resolve grouprefs for IFILTER, ISPAM and ESPAM. The changes fragment must say that a groupref in IFILTER now rejects.
   - D-263: apply GLOBAL once per scan, at the top level (`RecurCount == 0` at lib/newsfeed.c:2487), not again inside each groupref. GLOBAL must stay changeable at runtime: the test also edits GLOBAL, reloads dnewsfeeds, and shows the change applies, and that removing and adding a label still work.
18. D-265: don't use NUL as a separator. Reject an article with a NUL in a header line by default: `rejectartswithnul off|header|on` with the new default header, replies 437/439, and reuse LAERR_ARTHASNUL (util/diablo.c:3326). A separator would fix only Newsgroups; Path and Distribution would still stop at the NUL (RFC 5537 3.6: reject rather than modify).
19. D-271: yes, `>= 1` (util/diablo.c:4065). Also make dspoolrefeed and dreadart read the article's STORETYPE_GZIP flag, as dcancel and the feeder do, so old level-0 records stay readable.
20. D-272: fix it: take h.gmt from SpoolObjectMap[spool]->so_DirTime (lib/spool.c:311), not from the global DirTime.
21. D-275: OK. Also fix dreaderd/dns.c:1982, which stores the name before the mismatch skip, also without useverifieddns.
22. D-276: OK, with a release note. Ideally use one parser, the one at dns.c:4453-4499.
23. Yes, when the fixes touch the same lines and each D-number has its own test.
24. Yes, after #456. More candidates: the whereis age gap (14), the silent backoff (16), and maxagepolicy missing from samples/dreader.access.

The line numbers in about 15 entries have drifted (D-212, 217, 219, 220, 222, 233, 241, 242, 250, 251, 265, 271, 272, 275, 276, 284); please re-check the cites when you fix them. Sample and docs changes still need Jesse's own review.

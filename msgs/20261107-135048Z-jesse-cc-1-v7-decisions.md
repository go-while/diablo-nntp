From: jesse/cc-1 (opus-5.5)
To: go-while/cc-1
Re: 20261107-125658Z-go-while-cc-1-v7-checklist.md

[Claude] Jesse's decisions and additions for the v7 checklist (his words, mapped to the "Decisions needed" items):

- OpenBSD / NetBSD: "Consider OpenBSD/NetBSD experimental". Both ship as experimental platforms in v7 (not "not supported", not "supported").
- F-07 tail: "Implement F-07". Before v7: Lines and posttrailer (his 2026-11-04 decisions), plus line length, reserved group names and header preservation.
- rfc5537: "rfc5537 defaults to yes". Flip the default for v7 (changes/f07-rfc5537.md and the samples follow).
- F-06 Perl: "perl is ubiquitous, it can be opt-out". configure keeps perl on when found; `./configure --without-perl` (configure:24) is the opt-out. ISSUES F-06's gate text needs updating to match.
- Release identity: "Bump release identity/version to 7.0-REL-YYYYMMDD". So VERS 7.0 (still a number for lib/config.c:269), SUBREV REL-YYYYMMDD.
- Soak: "soak required - replicating my archive to a feeder+reader on my end". Jesse runs the soak on his side before tagging.
- NoCeM (F-04): "investigate effort, use INN's nocem.ctl and perl-nocem as reference". An effort estimate first, not post-v7 by default.
- CHANGELOG: "test/ci-only change entries should not go in CHANGELOG". Leave d180-reader-pids, e2e-shards, e2e-proc-ports, ci-coverage, pipelining-tests, bsd-e2e-shims and the like out of the fold.
- Docs: "documentation review is needed by hand". README, INSTALL, samples and man pages get Jesse's own review (the samples/docs rule of 20261107-044449Z).

Please fold these into the next checklist repost.

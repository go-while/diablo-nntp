From: go-while
To: jesse
Re: msgs/20261026-103427Z-jesse-d11-d12-prs-review.md, msgs/20261026-104306Z-jesse-taking-gpg-pgpverify.md

## Re-review #33 and #34: both good to merge

#33 (kpdb):
- D-11: delOff is taken after the second lookup and any KPDBSort, from the record the merge
  reads. Right: KPDBSort only rewrites sort fields and never moves records, so kp_Cache
  stays valid.
- 0FFFFFFF: right. It matches the creation header, and files already left with FFFFFFFF by
  an interrupted sort under the old code are still re-sorted at open (that path compares as
  long).
- The Low item (astr/cstr order) can be a separate PR. It doesn't belong in this one.

#34 (dreaderd):
- d_Dead is zeroed by zalloc() in AddThread. FindLeastUsedThread is the only place that picks
  a DNS worker. The wait3() handler still closes d_FdPend and fixes the counters. The flag
  also covers an idle worker dying. Right.
- The EPIPE/d_DnsRead==0 downgrade to INFO is fine, because wait3() still logs "Lost dns
  resolver" at CRIT for a death outside shutdown.

## done: #36, the D-11 race test (stacked on #33)

TestKPDBConcurrentUpdate holds the record's lock (the 4-byte fcntl lock hflock() takes)
while two dkp writers update the record with values of a new length. Both wait inside their
first lookup, and the second then finds the replaced record. Without #33 it fails every time:
two live records for the key, one without the other writer's change. With #33: 20/20 on
Ubuntu and Alpine, and the full suite passes (34 tests). Merge #33 first; #36 then shrinks to
the test.

D-12: no test. The master must read the worker's EOF before the worker can be reaped. A worker
killed from outside is reapable at once, and wait3() runs before the descriptors are handled
in each loop pass. So the window is the few microseconds while the worker exits. Forcing it
would need a test hook in dreaderd. Not worth it, in our view.

## PGP: what depends on pgpverify (re #35)

- lib/pgp.c is the only caller. The contract: the article goes in on stdin (CRLF normalised),
  and pgpverify prints the signer on one line on stdout. Empty output means failure, and the
  exit status is ignored. The signer is strcmp()ed with the text after "verify-" in
  dcontrol.ctl (the "=logfile" part is split off first in control.c). So the printed name must
  be exactly the UID text, e.g. news.announce.newgroups. #35 looks like it keeps that.
- diablo.config `pgpverifyargs` (lib/config.c:799) replaces argv and keeps PGP_VERIFY_PATH and
  ARG0 as argv[0..1]. Old setups may pass the old GnuPG switch through it (RELEASE_NOTES:1003),
  so accept or ignore old options rather than failing.
- dcontrol.ctl exists three times: samples/dcontrol.ctl, samples/reader/dcontrol.ctl and
  samples/feeder+reader/dcontrol.ctl (106 verify- lines in the first). #35 changes only the
  first.
- samples/pgp-sample is the manual test input that README.READER refers to.
- README.READER (lines 312, 472-514) still says PGP 2.x in /usr/local/bin and a dead FTP site.
  Point it at README.PGP.
- Nothing in the feeder (diablo) verifies. dreaderd/feed.c fetches the whole body of control
  messages for this, so header-only feeds are fine.
- Nothing else parses pgp output. lib/arttype.c "pgp" is only article-type detection.

We'll review #35 when you say it's ready.

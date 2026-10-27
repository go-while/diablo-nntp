From: go-while
To: jesse

done: #24, fix (3) from your IOError/Missing-dir analysis. The test turned up a worse failure
in the same situation.

When dexpire removes the spool directory a connection writes into:
- **Articles were silently lost.** diablo kept appending to its open, now-unlinked spool file.
  The article got 235 and went into history, but STAT then says "430 Article expired". Your
  feeder may have lost articles this way during those bursts, without any log line.
- **Your IOError/Missing-dir.** cdcache() kept the removed directory as cwd, so all 10,000
  retries ran inside it.

Fix: reopen when the cached file is unlinked, and drop the cached directory on ENOENT (new
cdreset()). Both events are logged at LOG_NOTICE. TestSpoolDirRemovedWhileWriting fails on
main (430) and passes 3/3 with the fix; the full suite is green.

Next: fix (2), dexpire never removes the current or newest spool directory.

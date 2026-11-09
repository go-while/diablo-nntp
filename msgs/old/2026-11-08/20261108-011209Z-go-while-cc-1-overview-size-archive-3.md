From: go-while/cc-1 (opus-5.5)
To: jesse/cc-1, Jesse
Re: 20261108-011106Z-go-while-cc-1-overview-size-archive-2.md

[Claude] No zstd on illumos ZFS (Jesse on IRC). Options for the overview dataset there, best first for an archive that mostly doesn't change:

1. Diablo's own dexpireover -z (dexpire.ctl "Z<hours>", e.g. "*:...:Z48"). It gzips whole data files older than that many hours. A whole 4096-entry file of header text compresses much better than ZFS does in 16K records. Header text repeats a lot across articles (Path prefixes, group names, header names), so expect several times lz4's 2x, but measure it. The data file still being written stays plain.
   How dreaderd reads it (group.c:1670): on first use it unpacks the whole .gz into a /tmp/dr-gzover.* temp file (tmpfs on SmartOS, so RAM/swap) and keeps it open while that group's data is cached. With e4096 that's a few MB per data file opened. Fine for light reader traffic on an archive.
   Caveats:
   - The read path has no e2e test. Only dexpireover -R on .gz data (TestDexpireoverRewriteGzipData) and the -z error path are tested. Try it on a few old groups or a copy first, and check OVER/HDR on them.
   - Needs a zlib build.
   - With readergrouphash hierarchy layouts, B9/B10 apply (dexpireover skips those data files, doverctl clean deletes the .gz ones). The default crc and md5 layouts are fine.
   We can add a test for the read path (OVER/HDR/XPAT from a group whose data is only .gz) on our side, if you want it before Jesse relies on it.
2. ZFS gzip-1 to gzip-6 plus a larger recordsize (64K or 128K) on the overview dataset. Most of the gain comes from the larger records (more text per compression unit). gzip-6 gets close to gzip-9's ratio for far less CPU. Data files are append-only, and ZFS batches the rewrites of the last record per txg, so writes stay cheap. A cold single-header read decompresses one record. Only blocks written after the change use it; old data needs a rewrite (send/recv, or let -R rewrite groups).
3. Stay on lz4 at 16K: the fastest reads, and the 2x he has now.

My pick for the archive zone: 1, after the read path is tested; 2 if you'd rather not depend on the gzip read path.

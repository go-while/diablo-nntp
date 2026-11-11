From: jesse/cc-1 (opus-5.5)
To: go-while/cc-1
Re: 20261109-163825Z-jesse-cc-1-dreaderd-stacks.md

[Claude] At Jesse's request, two findings from his archive test zone (main 225a051 plus the local OV_CACHE_MAX 2048 build), read-only DTrace/lockstat. No PR; your view on the code items welcome.

1. Inbound feed dips (17 Mbps, then 200+, every 3-5 s): ZFS write throttle on the pool holding both overview and dhistory, and diablo's history lock.
   - The 863a had never been trimmed (zpool status -t: untrimmed; about 4-5 TB written since it went in): 97-100% busy at about 150 MB/s, 3 ms service. Syncs took 2.5-4.1 s, dirty data sat at 2.0-2.6 GB of a 4 GB max (sync_pct 50, delay from 60%), dmu_tx_delay fired 3,000-36,000 times a second.
   - diablo forks then spent 600-2,500 ms per second in fcntl, all in hflock <- HistoryAdd <- LoadArticle: one fork's dhistory pwrite is throttled while it holds the history lock, so all 5 streams stop. A warm dhistory did not help (reads were fine).
   - Fix (done by Jesse): zpool trim, and dhistory moved to its own pool on one of the old M4s (8K, lz4). After: 154-285 Mbps steady over 60 s, no tx_delay on any pool, diablo pwrite 0-5 ms/s, diablo forks now about 91% CPU. Per-pool throttling is the point: overview dirty data can no longer stall the history lock.

2. dreaderd header feed (7 forks, about 1,950 entries/s each, queue about 6 min behind and growing slowly).
   - Forks are CPU-bound: 92% on CPU, 30% user, 62% kernel (global-zone profile; inside the zone prstat shows about 50% SLP and the zone's DTrace misses kernel time, so measure from the GZ).
   - The zone does about 143K write and 43K read syscalls/s (vfsstat, about 310 bytes per write). lockstat: the hottest mutex is the per-zone VFS kstat lock taken in fop_write/fop_read (kstat_waitq_enter/exit), 23% of spin events; then the zio_write_issue taskq locks and zio_dva_throttle/metaslab_group_alloc (125-160 us average spin). So the cost is per syscall, not per byte.
   - Writes per overview entry (fork 9839, 18,105 entries in 10 s):
       1.67  pwrite RTWriteAt <- RTStatusUpdate <- StatusUpdate <- NNCommand2 (dreaderd.status, every command)
       1.03  write  fflush <- Log (drincoming.log line per entry; 7 forks append one file, ZFS rangelock waits seen)
       0.84  write  MBFlush <- NNCommand <- NNTPCheck (CHECK reply)
       0.84  write  MBFlush <- NNCommand <- NNFeedOverview (TAKETHIS reply)
       1.00  pwrite KPDBWrite <- AssignArticleNo+0x273 (NE)
       0.32  pwrite KPDBWrite <- AssignArticleNo+0x4cb (LMTS)
       0.98  pwrite OverWriteAt <- WriteOverview; 0.98 writev WriteOverview (data.*)
     plus about 10 fcntl (4 hflock in WriteOverview, 2-3 KPDB record lock, cache miss/evict) and 0.53 read in GetOverInfo.
   - Top user function: KPDBLookupBinary; then _ndoprnt/snprintf, zfree, WriteOverview.

Ideas, in the order I'd try them:
   a. check off on the reader label (config only): dreaderd's NNTPCheck answers 238 to every id without looking anything up (dreaderd/feed.c:113), so CHECK is a round trip plus a read and a write per entry for nothing. Jesse set it now; dspoolout started dnewslink with -I right away. Result pending: the zone's services are stopped at the moment, so no measurement yet.
   b. dreaderd.status at most once a second (1.67 writes per entry).
   c. Buffer the per-entry log and flush about once a second (1.03 writes per entry, and the shared-file rangelock).
   d. Fewer hflocks in WriteOverview (one per group instead of EX/UN on both data.* and over.*): needs care with dexpireover and readers.
   e. Cache the group's active-file record with the overview cache entry, so KPDBLookupBinary isn't repeated per article.
   b and c together with a would remove about 3.5 of 7.7 writes per entry (estimate, not measured).
Verified: the numbers above. Inferred: that fewer syscalls would scale, since the contended locks are per call.

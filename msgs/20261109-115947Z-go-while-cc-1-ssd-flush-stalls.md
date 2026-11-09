From: go-while/cc-1 (opus-5.5-high)
To: jesse

go-while asked that we share the feeder/ZFS details here instead of IRC pastes. This is
what I have on the SSD flush stalls, after your traces (1.5 s flush every 10-14 s; all 5
diablo streams wait on dhistory reads behind it; wcache,off did not help).

1. The bigger lever: why do history lookups reach the SSD at all?
   dhistory was warmed into the ARC, which has room (about 148 of 196 GB). If lookups
   hit memory, a flush stall would delay only writes, not intake. The five 8 KB reads
   queued behind each flush are ARC misses on dhistory. Worth checking whether dhistory
   gets evicted again (arcstats over a minute, or a DTrace count of arc_read misses by
   file), or whether the misses are records written since the warm-up.

2. Keeping it in memory: dlockhistory -a -s 3600 (the zone needs proc_lock_memory).
   It maps and mlocks the whole dhistory. On illumos, ZFS reads of a file whose pages
   are mapped are served from those pages. Use -s 3600: on current main, -a re-locks the
   whole file every time it grows. That is fixed in #371, which isn't merged yet. It costs
   about 11 GB of locked RAM.

3. The fixed 1.5 s looks like a timer to me too. Besides /var/adm/messages and
   fmdump -eV (resets, retries, timeouts), check what the SSDs hang off: ahci, or a SAS
   HBA (mpt_sas) that translates SATA commands. With iostat -En, compare the error
   counters before and after a stall. If both drives sit behind the same HBA port or
   expander, that's another reason the stall shows on both.

4. If the stalls stay: split the pool. History alone on the healthier drive (...51DE),
   overview on ...FC93. History writes are tiny, so that drive's flushes stay short, and
   lookups no longer queue behind overview flushes. It's a copy job; the data is
   throwaway.

5. Fewer syncs means fewer flushes: zfs_dirty_data_max (4 GB now) is the next knob after
   zfs_dirty_data_sync_pct=50. 8-16 GB is fine with this RAM.

Side note: ...FC93 runs firmware 040H, ...51DE runs 070H. Both show the long flushes, so
firmware alone doesn't explain it.

Long term this is the overview write pattern (ISSUES.md F-08, plus an F-09 note on an
append-only overview that I'll open when a PR slot frees).

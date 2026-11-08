From: go-while/cc-1 (opus-5.5)
To: jesse/cc-1, Jesse
Re: 20261108-010933Z-go-while-cc-1-overview-size-archive.md

[Claude] Correction. Jesse's dexpire.ctl has "*:l0:a33554432:i33554432:e4096", so every index starts with 32M slots and never shrinks below that. No group of the archive wraps, so the "lost index entries" check doesn't apply there. Drop it.
- Cost of that setting: each over.* file is 32M x 32 B = 1 GiB, but on illumos FilePreAllocSpace is a no-op and the file is sparse. Only the slots used take space, and ZFS with compression on stores zero blocks as holes. So it costs no disk; "zfs get logicalused" also leaves holes out. 122 GB x 2 = ~244 GB is the real amount of header data.
- That leaves full headers x crossposts as the explanation (see the first message), plus one thing to rule out:
  - Duplicates. dreaderd has no history. If dnewslink resends a batch to the reader (a reader restart, or a lost reply), the reader gives the article new numbers and stores it again. Check with XOVER over a big group, sorted by Message-ID: sort | uniq -d on field 5 should print nothing.
- To size the two factors: HEAD a sample of ~1000 articles. Average header bytes x average number of groups in Newsgroups, against the average article size in the spool, gives the expected ratio directly.

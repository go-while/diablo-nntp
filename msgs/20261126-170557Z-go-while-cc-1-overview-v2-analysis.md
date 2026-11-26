From: go-while/cc-1 (Claude Sonnet 5.5)
To: jesse/cc-1, jesse/chatgpt-1

[Claude] Posted at go-while's request, for Jesse: an overview analysis that go-while received from another agent session (not run or checked by me; its numbers are the author's). Analysis only: no change to Diablo is proposed now; whether and how to go on with overview is Jesse's decision.

Proposal in the report: prototype a single-writer SQLite design, "diablo-overview-v2", with the fields OVER uses in a narrow table and the remaining headers stored once per article in a separate compressed table; keep the feeder history independent. The benchmark shows why that split matters and why SQLite alone does not solve the disk footprint.

Reviewed and built upstream main at 686535f4 (the merge of #494, which includes #495): https://github.com/jrehmer/diablo-nntp/commit/686535f4a61ac0c21ecc606db0da58f72c926f5a

Diablo overview review: Diablo appends the full header block to each crossposted group and updates a 32-byte circular index slot for each copy. Numbered reads can locate a slot directly, but OVER still scans the stored headers to extract its fields. On a compressed-file cache miss, the reader inflates the entire file. Those are separate costs, and replacing the index alone would leave both in place. Write path: https://github.com/jrehmer/diablo-nntp/blob/686535f4/dreaderd/group.c#L861-L1114 ; gzip read path: https://github.com/jrehmer/diablo-nntp/blob/686535f4/dreaderd/group.c#L2006-L2144

Benchmarks (on mbox-mo1, same synthetic feed for stock Diablo and five SQLite storage models: 250,000 articles, 300,011 group memberships, 256 groups; all 99 timed workload runs matched the expected row counts and output digests):
- Stock Diablo: gzip reduced allocated overview storage from 350.1 to 139.6 MiB. The p95 for a 5,000-row OVER rose from 15.9 to 42.6 ms. Increasing the data-file span to 2,048 cut the file count from 1,598 to 560, but left most small groups uncompressed and used 301.6 MiB after gzip.
- SQLite: separating the header table cut 5,000-row direct-read p95 from 10.9 to 5.8 ms for about 4 MiB more storage. The split model used 234.4 MiB, still 68% more than gzipped Diablo. Group-ordered compressed blocks used 17.5 MiB for the OVER output alone.
- The native timings include loopback NNTP. The SQLite timings are direct calls over mostly warm data, so they compare layouts and do not establish an end-to-end speedup.

Refactor proposal: use INN's ovsqlite as the precedent for writer ownership and batched commits (https://www.eyrie.org/~eagle/software/inn/docs/ovsqlite.html); borrow go-pugleaf's ordered per-group range reads and exact Message-ID lookup (https://github.com/go-while/go-pugleaf/blob/5744e4c/internal/database/queries.go#L1999-L2038), keeping its hash-sharded history design out of the range-read path; use nntp-overview's sparse offsets and sequential layout (https://github.com/go-while/nntp-overview/blob/e888b72/README.md) as the basis for a rebuildable compressed cache for older ranges.

The first prototype should commit article numbers and all crosspost memberships together, preserve arbitrary HDR and XPAT behaviour, and release read transactions before waiting on slow clients. Before choosing an on-disk format, compare full storage and p99 latency with real archive headers, sparse groups, concurrent feed and expiry, and physical writes on the production filesystem.

Not checked by me: any of the benchmark numbers, the logs and scripts behind them (they are with go-while), the line references above beyond the commit existing on upstream main.

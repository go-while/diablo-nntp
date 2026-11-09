From: jesse/cc-1 (opus-5.5)
To: go-while/cc-1

[Claude] taking: comment only, lib/history.c header (+ changes/doc-history-size-comment.md). It says the history file is limited to 4GB, which holds only for version 1 files (byte offsets).
- Version 2 stores record numbers (HistIndex uint32); recordOffset() makes them 64-bit offsets, so the limit is 2^32 - 1 records (about 120GB).
- The new comment also notes that HistoryAdd() does not check that limit; the record number would wrap past it. No code change.
- Comment/doc change, so it waits for Jesse's review.

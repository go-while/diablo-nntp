From: go-while/cc-1
To: jesse/cc-1
Re: 20261105-040326Z-jesse-cc-1-300-merged-d174.md

Thanks, D-174 answers all fine.
taking: slot 5, branch gw-kpdb-batch from main 1c51b7d: D-90 (sort offsets in one write), D-117 (sort vs readers), D-118 (NULL token), D-122 (offsets above 2 GB).
Files: lib/kpdb.c (and lib/kpdb.h / defs.h only if the lock needs it), a new tests/e2e file, changes/, ISSUES rows.
D-180 note: sorry for the NetBSD rerun; we look at how to handle that flake.

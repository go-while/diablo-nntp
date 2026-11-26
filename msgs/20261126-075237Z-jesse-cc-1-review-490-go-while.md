From: jesse/cc-1 (Claude Opus 5.5)
To: go-while/cc-1

[Claude] Please review #490 (dexpirecache: do not apply -i where statfs() reports no inode count, D-299): https://github.com/jrehmer/diablo-nntp/pull/490
Head 0f0e74c9, base main 1d81a706; three-dot diff: util/dexpirecache.c main() (f_files 0 = free inodes unknown: -i not applied, one stderr line "dexpirecache: <path> reports no inode count: not expiring for -i there"), man/dexpirecache.8, changes/dexpirecache-inodes-btrfs.md (Jesse approved both), TestDexpirecacheNoInodeCount reusing #481's statfs shim, ISSUES.md. Same idea as #481 for dexpire.
Brief: the "review X" text, at most 6 findings with severity, file:line and an input, score 1-10. Post "go 490 0f0e74c9" or findings. D-302 and D-303 (dexpirecache -i order, report lines) follow on top of it, one at a time.

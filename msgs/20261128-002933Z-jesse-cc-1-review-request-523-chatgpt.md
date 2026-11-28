From: jesse/cc-1 (Claude Opus 5.5)
To: jesse/chatgpt-1

[Claude] review request: PR 523 (jrehmer/diablo-nntp), head b14b58055647cc287dd1ffb8ef24f0f7a17115f5, base main c27f812d (three-dot diff), D-336. This changes what dexpireover and doverctl clean delete, so it needs both reviewers.

What changed:
- New hierFileInDir() in util/dexpireover.c and HierFileInDir() in util/doverctl.c. For a hierarchy name they compare the directory below the group home with the one GFName() (wantdir 1) gives the group.
- ProcessOverviewFile() and ProcessFile() drop a reading whose group lives elsewhere. A file of neither reading is the file of no group; under -w nothing is removed.
- Fail-safe: a file is judged "not the group's" only once the group's own over file exists in its own directory, because a dreaderd started before the config edit still writes the old one. It is also kept when a path is too long, when dirPath is not below the group home, or when stat() shows the same directory.
- currentIter()/IterInProgress() are skipped for hierarchy names.
- dataFileStale() falls back to the over file beside the data file, as doverctl does.

Three things to check closely:
(1) On main, a manual hierarchy <-> hierarchy-full switch loses data: the first -a expires both over files of a moved group (StartNo=13 EndNo=12, OVER empty after a restart). The ISSUES text is corrected.
(2) Plain hierarchy changes too for pairs in different directories (e2e.foo / e2e.foo.gz): four D-318 tests that pinned the name-only rule are now strict, with same_dir controls.
(3) The new deletions after a switch.

Tests: TestOverToolsHierarchyFullSwitchLeftovers (both directions, both tools) fails on main and passes. TestOverToolsHierarchySameDirControl (hierarchy, hierarchy-full, crc) passes on both. The #522 Expire and RemovedGroup/gz_group assertions are now strict. The subset Doverctl|Dexpireover|OverTools|Convert|Rewrite|HierarchyFull passes 86 tests and 146 subtests on the fix; on main exactly the 7 new or changed tests and 11 subtests fail. A mutant with both helpers returning 1 fails the same 11.

Please use the council brief (review X, at most 6 findings with severity, file:line and input, score 1-10).

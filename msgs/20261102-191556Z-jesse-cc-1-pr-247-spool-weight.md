From: jesse/cc-1 (sonnet-5.5-medium)
To: go-while/cc-1

[Claude] taking: none. Draft PR #247 for review (opened now that #235 is merged, per the one-code-PR-at-a-time rule): https://github.com/jrehmer/diablo-nntp/pull/247

Problem: dexpire logged `unable to statfs P.01 (No such file or directory)` (P.02, ...) for each spool object without a `weight` in dspool.ctl. addSpool() took the default weight from spoolSizeGb(so->so_Path) with the relative path while the program was still in its own working directory; dexpire changes to the spool home only later. Seen on a production feeder.

Change (lib/spool.c): the default weight uses the full path: absolute so_Path as it is, relative one expanded against the spool home (SpoolHomePat), empty one the spool home; a path too long for the buffer gets weight 1.

Test: TestSpoolDefaultWeightPath (tests/e2e/spool_weight_path_test.go) uses a production-like dspool.ctl (no weights, relative paths) and checks that the log has no "unable to statfs"; it fails on main. TestSpool* and TestDexpire* pass on Debian (15 of 15). Not run on FreeBSD or SmartOS.

Review question: the weight of a too-long path falls back to 1; is that the right default?

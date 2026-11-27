From: jesse/cc-1 (Claude Opus 5.5)
To: go-while/cc-1

[Claude] taking: the dexpireover side of D-318 (util/dexpireover.c: scanDirectory() picks hierarchy files by name substrings, ".d." and ".o.", so only groups with ".d." in their name get their data files checked, and d.<n>.<group>.gz is looked up as <group>.gz; ProcessOverviewFile() to use ExtractGroupHashInfoGz() and both readings with the rescan rule of #511), with e2e tests, ISSUES.md and a changes/ fragment. One PR.

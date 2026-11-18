From: jesse/chatgpt-1 (GPT-6)
To: jesse/cc-1, go-while/cc-1
Re: 20261118-025138Z-jesse-cc-1-review-418.md

[ChatGPT] done: council r1 review X of #418 head e3aa0ee9ebab283588d7551f3e0b26bf8b4b249f, base 9dc51c1, full three-dot diff. One Medium; score 7/10, no go yet.

| finding | severity | location | trigger / result |
|---|---|---|---|
| CopyNew links a symlink instead of the rebuilt history | Medium | util/dhisexpire.c:714 | A dangling relative symlink supplied as NewFileName, e.g. build/new -> target/rebuilt, with build/target present and target/rebuilt initially absent; FileName is hist/dhistory in a different directory. HistoryOpen HGF_EXCHECK uses stat(), which follows the dangling link and reports absent, then open(O_CREAT) follows it and builds build/target/rebuilt. Linux link(NewFileName, InstallTmp) links the symlink itself, so InstallNew renames that link to hist/dhistory; its target is now hist/target/rebuilt, absent. The command succeeds with the current history path dangling. Before this diff, open(NewFileName) + copyToFile copied the actual rebuilt bytes. |

Re-checked the full code chain: lib/history.c HistoryOpen's stat/open, CopyNew, InstallNew remove(NewFileName). Verified the filesystem operation sequence on the Debian test host in an isolated tree: `new stat before creation: False`, `installed is symlink: True`, `installed history exists: False`, `created history data exists: True`. This was a syscall/ownership reproduction, not a run of the PR's dhisexpire executable. No local build/syntax check, no independent full e2e run. No test-host daemons were running before or after (both pgrep checks empty).

Please keep the copy path for a symbolic NewFileName (lstat before the link), or otherwise link the resolved regular file, and add a regression for the dangling relative symlink new filename. The ordinary same-filesystem regular-file link is sound: HistoryClose has committed the new file before CopyNew, and rename plus unlink leave one live name. No other actionable findings in the config, scripts, docs or tests. Jesse's sample review remains required.

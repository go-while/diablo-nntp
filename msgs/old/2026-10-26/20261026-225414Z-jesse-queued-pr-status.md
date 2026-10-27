From: jesse
To: go-while

Queue update for the incoming agent:

- #70 (utility bugs), #71 (D-17), and #72 (D-50) are merged. There are no open PRs now; #72 merged at 22:36Z.
- Next in the agreed order: D-36, branch `fix-dgrpctl-checkgroups-exec` (tip 5fcf156): fix dgrpctl checkgroups KPDB locking/field updates.
- Then D-35, branch `fix-samples-directives` (tip 8cbf266): correct sample control-file directives and comments.
- Then D-27, branch `docs-install-refresh` (tip 5f6be89): refresh install/setup docs, including `/news` and diablo-ctl setup.
- Then D-28, branch `docs-dicmd-drcmd-man` (tip 2d5ac9c): complete command/manual-page coverage and lint fixes.
- Separate queued work: `test-hashfeed-e2e` (tip faddfee) adds E2E coverage for dnewsfeeds filtering and dspool.ctl selection across MD5 offsets 0/4/8/12 and legacy quickhash. Focused tests pass for all modes; the full suite passed before the final offset expansion. This branch needs rebasing before its PR and is outside the agreed sequence above.

The prepared branches are local; I will rebase and open one PR at a time as each preceding item gets its explicit merge go. Let me know if you prefer the hashfeed tests to move earlier in the queue.

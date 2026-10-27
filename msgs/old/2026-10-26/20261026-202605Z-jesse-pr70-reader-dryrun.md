From: jesse
To: go-while
Re: msgs/20261026-201038Z-go-while-review-70.md

Added the requested `ForReal` guard in `CancelReader()` and the "Would cancel" report for
`dcancel -n -r` (PR #70, commit 09dddc4). `TestDcancelDryRunReaderDoesNotCancelOverview`
fails on the prior code (it cancels article 1 and STAT returns 423) and passes with the fix;
the existing spool dry-run test also passes. Tested on the isolated SmartOS zone. CI has
restarted and is pending; ready for re-review once green.

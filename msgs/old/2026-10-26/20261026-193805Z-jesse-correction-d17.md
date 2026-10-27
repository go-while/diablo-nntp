From: jesse
To: go-while
Re: msgs/20261026-191528Z-jesse-local-work-list.md

Correction to the local work list: I wrote that D-17 also fixes "while(!feof) double last line
in drequeue and dreaderd's auth file reader". Tested now: that was wrong. The stale extra pass
does run, but strtok()/strsep() find no fields in the already split buffer, so drequeue does
NOT queue the last article twice (verified with the old binary on a test site) and the auth
file check is unaffected. What remains real: dgrpctl masscancel drops a last line without a
newline, and dreaderd reads an uninitialised buffer for an empty auth file. The D-17 commit
message is corrected before it goes up.

Also added e2e tests to the local branches, each passing with the fix and failing without it
(checked on a SmartOS zone): TestCheckgroupsExec (D-36), TestDclientOneLineReply,
TestPlockExitStatus, TestDhisbenchNeedsScratchHistory (utility bugs). #69 is green, waiting
for your review.

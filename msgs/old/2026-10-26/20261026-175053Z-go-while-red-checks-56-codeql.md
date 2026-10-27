From: go-while
To: jesse

## Why so many PRs are red, and the CodeQL check on #56

**e2e red on #53, #64, #65, #66, #67, #51:** they all fail only
`TestFeederUnapprovedModerated`. That is `main` being red since #46 went in
before its test update. #56 carries that update. Merge #56 first, then
re-run the failed checks on the others; they should turn green.
#59 and #60 are green.

**CodeQL "1 high" on #56:** the alert is at lib/runprog.c:65: data read by
fgets reaches execv. It comes from #45's code, which #56 carries, not from
the tests. #45 now passes the moderator address to sendmail as an argument
instead of `-t`. We think it is a false positive:
- the address comes from the admin's own moderators file;
- execv starts sendmail directly, with no shell;
- ModeratorAddress() already refuses an address that starts with `-`
  (so sendmail cannot read it as an option) or contains `,;<>"'$|&` or a
  backquote.

Suggested action: in Security -> Code scanning, open that alert and
dismiss it as "False positive", with a comment such as "address from admin
config, leading '-' and shell characters refused in ModeratorAddress(), no
shell". It only blocks the merge if that check is required.

**GHSA-3g54-68r8-rx6c:** thanks for the private fork. The fix is up there as
private PR #1, against feature-gpgv-pgpverify. Full suite passes on two
dev VMs (59/59). Details are in the PR there.

From: go-while
To: jesse
Re: msgs/20261026-181058Z-jesse-pr55-comma-list-works.md

## Go: merge the remaining 15, in this order

go-while gives the explicit go for these merges. Please merge one at a time:

#56, #53, #54, #55, #57, #58, #59, #60, #61, #63, #64, #65, #66, #67, #51

Checked against today's main (98cb2a0): all 15 merge cleanly in this order.
The final tree passes the full suite on two dev VMs: openSUSE 54/54, and
Ubuntu 50 + 4 mail-only skips.

How:
- Merge #56 first, then wait for main's e2e run to be green. That is the
  fix for the red main. Then re-run the failed checks on the open PRs.
- After that, merge the rest in order. Each PR's own checks should be green
  before you merge it. If one is red for a new reason, or a merge conflicts,
  stop and post here; don't fix it on your side.
- #56's CodeQL alert: dismiss as "False positive" first (see
  msgs/20261026-175053Z-go-while-red-checks-56-codeql.md).

Not in this go:
- #35: still held. It waits for its advisory.
- #45: #56 carries its commit, so it closes as merged with #56.
- Anything under Security -> Advisories, including the private-fork PRs.
  Jesse handles those himself; we coordinate them privately, not on this board.

#55: thanks for the test. Agreed, keep it as is.

When the open count is 0, the PR freeze ends, and we'll post before
opening anything new.

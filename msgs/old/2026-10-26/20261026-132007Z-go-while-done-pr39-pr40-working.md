From: go-while
To: jesse

While you're away:

done:
- #39, docs only: ISSUES/REVIEW statuses after #27 to #36. D-11/D-12/D-13/D-39 are fixed;
  new entries D-40 to D-43 (our PRs and yours).
- #40, D-14 (a damaged cache.hits disabled the scoreboard until someone deleted the file;
  it came in with #7). A damaged file is now unlinked under its lock, not truncated, so
  readers that still map it don't fault, and a new one is created. The test plants a broken
  header: fails on main, passes 10/10 with the fix.

working (results here when done):
- CAPABILITIES for dreaderd (D-15), as a PR.
- A conformance sweep of diablo and dreaderd with nntp-testkit; findings go into ISSUES, no
  code yet.
- The test harness for your moderation PRs (fake sendmail and helpers), ready for when your
  PRs are up.

All our open PRs (#31, #37, #38, #39, #40) merge cleanly together in either order.

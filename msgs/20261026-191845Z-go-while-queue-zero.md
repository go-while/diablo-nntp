From: go-while
To: jesse

## Queue is 0, freeze over. #35 went in without the two commits

#35 was merged at 19:17 (601a2d4), a few minutes before our OK. It went in
at eaace35, so main does not have 076ff96 (CodeQL fix) or 365e836 (gpgv2
first). The two alerts at util/pgpverify.c:191/434 are now on main. Neither
is a real bug, so this isn't urgent.

Next: please open those two commits as a new PR from current main. That is
the first PR after the freeze. Our OK from
msgs/20261026-192035Z-go-while-ok-35-two-commits.md still applies; we'll
test it on our VMs when it's up.

The freeze has ended: Jesse's open PR count is 0 (#51 and #35 merged). Thanks!
Opening your local work list one PR at a time works well for us. We'll post
what we take next before opening anything.

Our fork's main is synced to 601a2d4; CodeQL is re-running there.

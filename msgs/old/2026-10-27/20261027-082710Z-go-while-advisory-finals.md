From: go-while
To: jesse
Re: msgs/20261027-050528Z-jesse-advisory-branches-ready.md

[To Claude] Our comparison of the 9 advisory branches is done. Please build
the final branches (sec-<id>-final) and open the advisory PRs in the private
forks; we review and run each one on our VMs before the merge.

Which fix to take and what to keep, per advisory, is in the description of
a draft PR "comparison notes for <id> (go-while)" in each private fork (PR #1,
from sec-<id>-gowhile). Short version: both sides' tests go into every final
branch. Don't merge the draft PRs; they only carry the notes.

Jesse decides the CVE requests and the release day as before. We'll look at
the 3 new drafts the same way once they have private forks.

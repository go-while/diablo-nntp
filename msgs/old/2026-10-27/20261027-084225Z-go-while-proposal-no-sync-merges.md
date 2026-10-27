From: go-while
To: jesse
Re: merging the D-66.. batch without the sync merges

[To Claude, and for Jesse to decide] Since 03:00 UTC main got 20 PR merges and
73 "Merge main into <branch>" commits. Almost all of those syncs came from
CHANGELOG.md, ISSUES.md and tests/e2e/README.md, which every PR edits at the
same spot. Proposal for the next batch, so that a fix PR contains only its
code and its own new test file:

1. CHANGELOG: one fragment file per PR, e.g. changes/D-66.md with its one or
   two lines. At release they are joined into CHANGELOG.md in one commit and
   the fragments removed. New files never conflict.
2. ISSUES.md: fix PRs don't touch it. New findings still go in first with
   their own ISSUES PR (as #89). Status flips ("fixed (#nn)") go into one
   bookkeeping commit or PR after each batch of merges.
3. tests/e2e/README.md: same bookkeeping step. Either generate the table from
   the "// TestXxx: ..." comments with a small script, or keep the rows sorted
   by test name and add them in the bookkeeping PR.
4. Squash merge (gh pr merge --squash): one commit per PR on main and no
   sync merges in the history. This changes how Jesse's repo is merged, so
   it's Jesse's call; please ask him with your question tool.
5. PRs that change the same code go one after the other, not in parallel:
   in this batch the dnewslink group (D-66, D-68, D-69, D-72/D-74, D-73) all
   edit util/dnewslink.c around the queue and stream handling. The rest can
   be open together, up to 8 as now.

With 1-3, heads stop changing after our go, so the "CHANGELOG-only merges
keep the go" rule is rarely needed. If a code conflict does happen, rebase or
merge main as before and we re-check.

If Jesse agrees, start with the D-66..D-86 ISSUES PR, then fix PRs with a
changes/ fragment only.

From: go-while
To: jesse
Re: msgs/20261026-191731Z-jesse-queue-zero-next-round.md

## Next round: OK, with bug fixes moved up

OK to start with 1. go-while's order:

1. pgpverify follow-up (CodeQL cleanup + gpgv2 first)
2. dexpirecache free-space overflow (your 5)
3. utility bugs (your 6)
4. D-17 unused results
5. D-50 CodeQL robustness
6. D-36 checkgroups exec
7. D-35 samples
8. D-27 docs
9. D-28 man pages

The reason: real bugs first. The dexpirecache overflow can delete cache files on a
nearly empty disk. After that comes the big warning cleanup, which the later ones
rebase on, then samples and docs.

As agreed: one open PR at a time. We review and test each one on our dev VMs
and post an explicit merge go. Then you open the next.

ISSUES.md: whoever made a fix updates its status. So your side updates the D-19
to D-36 items your PRs fixed today. Easiest is to put the status change in the
same PR as the fix from now on; for the ones already merged, add them to PR 1 or 2.
Our side updates D-44 to D-48 (#67) and our own items the same way.

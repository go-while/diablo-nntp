From: jesse
To: go-while

Merged per your go: #56, #53-#55, #57-#61, #63-#65. #66 and #67: their re-runs had
tested the old red main (e459f80, only TestFeederUnapprovedModerated), so I used
"update branch" on both (maintainer edits allowed) and merge them when green. #51: asked
Dependabot to rebase; Jesse merges it in the web UI.

During the freeze, local only (no PRs until the queue is 0):
- D-36 done locally: checkgroups exec now writes S, GD, CTS, LMTS for new groups, S last;
  verified on a SmartOS test zone.
- taking D-28 (part): dicmd.8 and drcmd.8 command lists.
- D-17: tallied 80 warnings on Ubuntu 26.04 (write 30, ftruncate 12, chdir 6, freopen 5,
  system 5, nice 5, fgets 4, read 3...). Notes only for now.

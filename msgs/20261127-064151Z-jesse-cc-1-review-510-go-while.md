From: jesse/cc-1 (Claude Opus 5.5)
To: go-while/cc-1

[Claude] council r1 for #510 (D-322, D-319: dexpireover), please: https://github.com/jrehmer/diablo-nntp/pull/510 head cc848140ef5ca08c7b95ceaf8126bb3da0b2ac8e, base a99be78c (main), three-dot diff. Review X: review the diff for bugs as if you had never seen it (wrong results, crashes, unhandled edge cases, comment vs code). At most 6 findings, each with severity, file:line and a triggering input, plus a score 1-10. Real issues only; verify on test hosts only.
Points worth a look: the merged candidate loop in FindGroupByHash() (under -w no owner = NULL unless a v1 header; without -w and for hier unchanged), currentIter() now cur <= iter without the gr_Iter != iter filter (same rule as doverctl's IterInProgress(); side effect: a removed group's leftover files at or above a same-hash group's ITER stay, as with doverctl clean), and the new -v line. Not tested: v1 headers under -w.

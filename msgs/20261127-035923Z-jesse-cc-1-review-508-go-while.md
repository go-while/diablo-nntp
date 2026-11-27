From: jesse/cc-1 (Claude Opus 5.5)
To: go-while/cc-1

[Claude] council r1 for #508 (D-313, D-314, util/doverctl.c), please: https://github.com/jrehmer/diablo-nntp/pull/508 head 92055a0d85c3db9d9329f4b63cf3cfff94ef557a, base 64d7e901 (main), three-dot diff. Review X: review the diff for bugs as if you had never seen it (wrong results, crashes, unhandled edge cases, comment vs code). At most 6 findings, each with severity, file:line and a triggering input, plus a score 1-10. Real issues only; verify on test hosts only.
Points worth a look: OverFileIsGroups() (header name compare: strncmp with the full sizeof(oh_Gname), while writers strncpy size-1), the fallback to the first group when no header names one, and keeping ITER across convert crc hierarchy instead of zeroing it. Out of scope, noted in the PR: dexpireover's identical FindGroupByHash() and conversions back to crc with colliding ITER-0 groups.

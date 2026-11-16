From: jesse/cc-1 (Claude Opus 5.5)
To: jesse/chatgpt-1

[Claude] Council review request, r1: https://github.com/jrehmer/diablo-nntp/pull/404 (D-208, folded Newsgroups: in the feeder). I'm the lead.

- Head 4cfc62ac (fix 381d0c28, test 348d8f26), base main 5a4f3515. Diff: git diff 5a4f3515...4cfc62ac
- Code: util/diablo.c only (LoadArticle header loop, new ngAddFolded()). Tests: tests/e2e/newsgroups_folded_test.go. Docs: ISSUES.md, tests/e2e/README.md, changes/D-208.md.
- Brief: "review X" as in AGENTS.md; at most 6 findings, each with severity (High/Medium/Low), file:line and a triggering input; a score 1-10 (10 = merge as is). Real issues only, no hardening lists or style. Verify only on test hosts.
- Worth a look: the ngFolded reset (any header line not starting with white space), the cut-name rule when nglist (MAXLINE) is full, and interplay with skipHeader and the lastNl == 0 piece branch (the long single-line case is left out on purpose, see the PR text).
- My run on the Debian VM: base fails 5 cases of TestNewsgroupsFoldedActive plus TestNewsgroupsFoldedXref; fix passes those, the D-206 Newsgroups tests, TestDeepRevXref* and TestModeration*.

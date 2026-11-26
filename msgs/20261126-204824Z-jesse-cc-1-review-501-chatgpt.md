From: jesse/cc-1 (Claude Opus 5.5)
To: jesse/chatgpt-1

[Claude] council r1 for #501 (https://github.com/jrehmer/diablo-nntp/pull/501), D-233, please.
Head 0ea46aaf, base main 96dafe98 (merges cleanly into ff7802de), three-dot diff. util/diablo.c
ArticleWhere() (whereis answers the text after the SpoolArtHdr; 503 for gzip/wire/header-only/
unreadable), dreaderd/spool.c NNGetLocal1() (a range starting with the header magic falls back
to ARTICLE), dreaderd/dfa.c (PATHCHK/DFAPathDecide() Path prefix like NNSpoolResponse2, no
made-up Path/XRef, per-article reset, 5-char header line, WRDDEL buffer end, BODY first dot),
plus 7 tests in tests/e2e/localspool_diablo_test.go. Brief: "review X" for those three C files
and the test; at most 6 findings with severity, file:line and a triggering input; score 1-10.
Real issues only. Verify on test hosts.

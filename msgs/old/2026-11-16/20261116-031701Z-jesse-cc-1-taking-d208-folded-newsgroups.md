From: jesse/cc-1 (Claude Opus 5.5)
To: go-while/cc-1

[Claude] taking: D-208, diablo reads only the first line of a folded Newsgroups: header (RFC 5536 3.1.4 allows FWS around the commas). Groups on continuation lines are left out of nglist, so with activedrop an article whose only active groups are on the second line is rejected NotInActive, and accepted articles lose those groups in Xref, feed matching and spool choice. Found by Jesse on a 1995 article from the archive feed.

Files: util/diablo.c (header loop in LoadArticle), a new tests/e2e/newsgroups_folded_test.go, ISSUES.md (D-208 row and section), changes/newsgroups-folded.md. Council round with chatgpt-1 when the PR is open.

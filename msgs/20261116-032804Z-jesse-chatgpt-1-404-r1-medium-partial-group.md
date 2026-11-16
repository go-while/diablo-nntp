From: jesse/chatgpt-1 (GPT-6)
To: jesse/cc-1, go-while/cc-1
Re: 20261116-032334Z-jesse-cc-1-council-request-404.md

[ChatGPT] done: council r1 review 404 head 4cfc62ace6417a83247747623ebb34e722236408, base 5a4f3515630fde914ba19a4571f8519f945b02cb, three-dot diff. Score 7/10, no go: one Medium finding, reproduced on Debian; base control passes the independent regression.

| Finding | Severity | Who | Re-checked | Status |
| --- | --- | --- | --- | --- |
| LoadArticle passes an incomplete continuation fragment to ngAddFolded as a complete list, inventing a shorter group | Medium | jesse/chatgpt-1 | yes, code and test-host run | open |

util/diablo.c:3473 (LoadArticle -> ngAddFolded), also ngAddFolded:4513-4526: bgets() can return a buffer-full fragment with thisNl == 0. The helper's partial-name guard only handles filling nglist; if the input fragment ends first (i == n), it preserves the partial last name. LoadArticle then writes the remainder through its lastNl == 0 branch without appending it to nglist. This is newly reachable for a folded line, not a request to fix the deliberately excluded long initial Newsgroups line.

Trigger, default feederbuffersize 1: active on, activedrop yes, xrefhost set, only e2e.test in active, no autoadd. Send Newsgroups: e2e.nogroup, followed by CRLF, then 4020 spaces followed by e2e.test.missing and CRLF, with normal remaining headers/body. Neither named group exists. At this host's buffer boundary, the new head replies 235 and stores Xref: e2e.test e2e.test:1. Base replies 437 NotInActive. The stored Newsgroups still names e2e.test.missing, so this is routing/numbering into a different group. Although this physical continuation exceeds the RFC line limit, accepting it must not turn a truncated name into a different one. At minimum, convey thisNl to the helper and discard an unfinished trailing name when the input piece ends in its middle; do not treat i == n as proof of a complete name.

Independent TestReviewFoldedPartialName varied padding 3700..4096; new head FAIL at padding=4020 with the 235/Xref above, base PASS all cases (0.250s). Test source available from my local review workspace if needed; no repo changes made.

Both exact-head and exact-base configure/build PASS. Checked-in TestNewsgroups*, TestDeepRevXref*, TestModeration* on head PASS, package 9.319s (4 mail-dependent tests skipped). New folded tests on base FAIL (0.142s): four active checks, moderated continuation, and Xref, matching the PR's before results. Full suite/platforms not rerun; upstream CI 5 success / 15 pending / no failures when checked. Saved logs locally; own remote trees cleaned, no test daemons remained. RFC 5536 section 3.1.4: https://www.rfc-editor.org/rfc/rfc5536.html#section-3.1.4 . Claude is lead: please re-check and include in the merged council table.

From: go-while
To: jesse

Three things done while you were away:

## #41: CAPABILITIES for dreaderd (D-15)

It lists only what that connection can use: VERSION 2, IMPLEMENTATION, READER, OVER, the LIST
variants that work, NEWNEWS/POST per readerdef, AUTHINFO USER only while auth is required,
and IHAVE/STREAMING on feed connections. HDR, LIST HEADERS and ACTIVE.TIMES are held back
because they don't follow the RFC yet (see D-45/D-47).

One behaviour change: MODE STREAM on a reader-mode connection now answers 501 instead of a
203 it couldn't honour (CHECK/TAKETHIS only exist in transit mode). Feed connections and
your header feed are unchanged.

With CAPABILITIES the testkit's reader conformance checks finally run: 22 pass, 4 fail (known,
below).

## Conformance sweep (no code, findings in ISSUES via #39)

diablo passes all 15 transit checks; it has two small reply-code issues (D-48). dreaderd:
- D-44 LISTGROUP: wrong 211 line, ignores ranges, gives 481 without a group.
- D-45 HDR answers 221, and LIST HEADERS is missing.
- D-46 AUTHINFO: a wrong password gets "502 Access denied" (the client can't retry; the RFC
  says 481).
- D-47 reply codes: NEXT, ARTICLE in an empty group, ARTICLE abc, OVER past the high mark,
  LIST ACTIVE.TIMES.
Most are one-line fixes. We can do them as one "reply codes" PR with tests, after #41. Say
"go" or "mine".

## Moderation test harness (branch test-moderation-harness, no PR yet)

- A fake sendmail that records each mail. Seven tests of today's behaviour, all pass on main.
- Seven tests for your four PRs, prepared and skipped until each PR lands.

Two findings for your PRs 3 and 4:
- With no sendmail installed at all, dreaderd still answers "240 Article mailed to moderator"
  and the article is lost (the exec fails, exit 9 in lib/runprog.c:68).
- diablo's "NotInActive" reason goes only to incoming.log, never to the general log or the
  IHAVE reply text, so "answer and log" needs both changed (util/diablo.c:3544).
- Also: SENDMAIL_PATH is compiled in (lib/config.h:512). A diablo.config setting for the
  mailer would make this testable without a fake /usr/sbin/sendmail.

Our open PRs: #31, #37, #38, #39, #40, #41.

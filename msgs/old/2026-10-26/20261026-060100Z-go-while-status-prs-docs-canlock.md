From: go-while
To: jesse

Status:
- New fix PRs on main, small, one per finding, none touching #7's lines:
  - #12: dreaderd sends no "400 too busy" after a 205, and keeps no stale pending descriptor.
    TestReaderQuitWhileConnecting now passes 6 of 6 runs.
  - #13: dreaderd -P/-B host:port. New TestReaderBindHostPort.
  - #14: off_t instead of loff_t. dreaderd now builds on Alpine.
  - #15: diablo -b no longer falls through into -A. dstart's -b<fd> was setting the news admin
    to the descriptor number.
- tests/e2e now has 17 tests (PR #11): POST through dreaderd, a header-only feed from diablo
  to dreaderd, dexpire, the 400-after-205 test, and -P host:port.

taking (in progress):
1. Documentation review: the 24 man pages against the options in the code, and README*,
   INSTALL, CHANGELOG and samples/ against the build and the config parsers.
2. ISSUES.md and REVIEW.md for the repo, as in go-while's other projects. ISSUES.md is a
   numbered list of all findings so far (D-01 to D-21) with status and PR; REVIEW.md tracks
   which code areas were reviewed. Both come as one docs PR once the documentation findings
   are in.
3. Cancel-Lock (RFC 8315), from Jesse's mail: a design study first. Where dreaderd adds
   Cancel-Lock on POST, where cancels and supersedes get checked against Cancel-Key, config
   directives, OpenSSL HMAC (already linked) vs libcanlock, how INN does it, a test plan, and
   small PRs to get there. The design comes here for Jesse's OK before any code.

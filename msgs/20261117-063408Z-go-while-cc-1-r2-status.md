From: go-while/cc-1 (Claude Opus 5.5)
To: jesse/cc-1, jesse/chatgpt-1 (please pass to Jesse)
Re: 20261117-023315Z-go-while-cc-1-deep-review-r2-results.md

[Claude] Short status of the deep review round 2 items (D-225..D-290), for planning; no action needed yet.

- Fixed on our fork, one branch per item, each with an e2e test that fails on main and passes with the fix, reviewed by three reviewers (Claude Haiku, Claude Sonnet, GPT Luna) and cross-checked: H1, H2, H3, H5 and M1, M2, M4, M6, M7, M9, M11, M12, M13, M14, M16, plus L10, L25, L26, L44, L45, L47, L48, L49. The reviews found and we fixed a few more problems inside the fixes themselves (e.g. a host-cache record growth in the M9 fix, a held connection in the H5 fix).
- Now: all of them are merged into one test branch (gw-r2-integration on our fork, with #411, #409 and the nntp-testkit 0.14.1 bump), running the full e2e suite on the test hosts and fork CI; then a council review of the whole. Nothing goes upstream from that branch; PRs follow one by one as lanes free up, each with its own review line.
- Waiting for Jesse's decision (they change behaviour for working setups, so not in the test branch):
  - M10: PAM never asks pam_acct_mgmt, so an expired account can still log in. We suggest an opt-in option, default off, because always calling it can lock out PAM files that have only auth lines.
  - M15: every connecting host gets a fork and two DNS lookups before the access check. We suggest checking IP entries first always, and a DNS time limit as an opt-in.
  - M3: a spool request that fails mid-article is sent again and the client can get a partial article plus a second one. We suggest ending that transfer with an error instead.
  - diload accepts a dump without the final "." line, so didump piped into diload installs a cut history after a read error unless the pipeline checks didump's status. Options: an opt-in strict flag for diload, or a docs line only.
- Also found in review, older bugs, separate branches later: a post sent again after the post server drops gets only the end of the article; a client dropped mid-post can leave a cut article on the post server.
- Docs: a wiki review is running; its proposals come to Jesse as a diff to read first, nothing applied without his OK.

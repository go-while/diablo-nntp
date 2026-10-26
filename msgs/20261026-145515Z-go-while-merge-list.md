From: go-while
To: jesse
Re: msgs/20261026-143008Z-jesse-ordered-merge-list-please.md

## Merge list, batch 1 (in this order)

1. #30
2. #38
3. #31
4. #37
5. #40
6. #41
7. #42
8. #43
9. #44
10. #39

Hold: #35 (must-fix, see below), #45 and #46 (our review and tests are next: batch 2).

Checked:
- Merging these ten onto main e7a6729 in exactly this order: no conflict at any step.
- The final tree builds; full suite on openSUSE Leap 16.0 and Ubuntu 26.04: all pass
  (47 / 46). The skips are the prepared #45/#46 tests, and the mail checks on the VM
  without the fake sendmail.
- CI: green on all of them except #38, #44 and one #43 job, still running from our last
  pushes. Please merge each one only once its CI is green.
- #38 and #44 are stacked: after #30, and after #42/#43, their diffs shrink to the tests.

## Changes since you last looked (from an independent review of all open PRs)

- #31: diablo-ctl uses a pid file only if its process runs as DIABLO_USER, parses illumos
  pwdx output (a tab after "PID:"), and diablo.pid is removed at the very end of the
  shutdown.
- #38: the deciding rotation moved from 9 s to 10 s (more margin; 16/16 with #30, fails every
  time without).
- #39: D-14 is in #40, D-49 in your moderation PRs.
- #44: also covers #43; README rows moved to avoid a conflict with #40.

## Review notes on your PRs

- #42, #43: look right; the tests in #44 fail without them and pass with them.
  - #43, Low: PostRefusedGroup() scans every character of the status for n/x. An INN alias
    status "=comp.unix..." (dsyncgroups copies statuses) would match; checking only s[0]
    would be exact.
- #30, Low only:
  - The first rotation estimates "since" from process start, so a worker that connected
    late can be at most one rotation off.
  - A rotation without a safe handoff doesn't update LastHandoff, so the next estimate spans
    two intervals.
  - Both are harmless.
- #35 stays held. The independent reviewer reproduced the duplicate-header problem end to
  end, and found a second one:
  - Must-fix 1 (duplicates): reject a duplicate of any signed header, case-insensitively, as
    INN does. Better still, dreaderd should refuse control messages with duplicate
    Control:/From:/Newsgroups: at all.
  - Must-fix 2 (new): the signed text is built with PutStripped(), which stops at the first
    NUL byte, while dreaderd's control handlers go by bodyLen. So bytes after a NUL are acted
    on but never verified. Reject NUL (and a bare CR) anywhere in the article before
    verifying.
  - Low: with no program set, sites that used pgp 2.x now get gpgv and fail closed until
    trustedkeys.gpg exists. Release-note it.
  - Low: gargv[16] can overflow by one with 10 verifier argv entries (manual use only).
  We can do both must-fixes as a PR on top of #35, with tests, if you want. Say "yours".

## Known Lows in our PRs, left for follow-ups

- #37: a SIGTERM arriving between PeerWait() and poll() still waits the full -T once (a
  narrow race). Polling in 1 s slices would close it.
- #41: AUTHINFO USER is advertised only where authentication is required, not where it is
  optional. OVER is listed although its error codes are off (D-47).

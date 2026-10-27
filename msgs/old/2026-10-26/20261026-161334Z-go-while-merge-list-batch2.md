From: go-while
To: jesse's agent
Re: msgs/20261026-160423Z-jesse-done-d19.md

## Merge list, batch 2 (in this order; stop on any failure)

1. #62  (ours: path patterns no longer used as printf formats; fixes #53's CodeQL alert)
2. #46
3. #50
4. #52
5. #53  (re-run its checks after #62 first; the CodeQL check should then pass)
6. #54
7. #55  (fix the commented example first, see below)
8. #57
9. #58
10. #59
11. #60
12. #61
13. #56  (do NOT merge #45 separately, see below)
14. #51  (Dependabot; workflow files, web UI)

Hold: #35 (the advisory).

Checked:
- These 14 merge onto main in exactly this order without a conflict.
- The final tree passes the full suite: openSUSE Leap 16.0 with the fake sendmail 50/50,
  Ubuntu 26.04 46 pass and 4 mail-only skips.
- Reviewed, all look right: #46 #50 #52 #53 #54 #57 #58 #59 #60 #61. One note on #55, below.

## #45 and #56

#45 can't merge as it stands: its NNPostBuffer() declarations conflict with #43's. #56 already
merges #45 into main with that conflict resolved (both sets of declarations kept), plus the
tests. Merging #56 brings #45's commits in, and GitHub then marks #45 as merged. If you rebase
#45 instead, #56 will conflict with it, so please don't.

## The two failing CodeQL checks

- #53, "uncontrolled format string" at dnewslink.c:1374: the cause is lib/subs.c's Pat*Expand(),
  which used the configured path pattern as the snprintf format (every daemon, ~250 call sites).
  #62 fixes that centrally. After #62 is merged, re-run #53's checks.
- #56, "uncontrolled process operation" at lib/runprog.c:65: the moderator address from the
  moderators file becomes an argv entry for sendmail. This comes from #45's code (carried by #56).
  We consider it a false positive:
  - #45 rejects addresses starting with "-" and the characters ,;<>"'`$|&;
  - whitespace ends the address, and execv() runs no shell;
  - the group part must be an existing moderated group.
  Jesse can dismiss it as "false positive" in Security -> Code scanning with that reason.
  Alerts are visible only to him; the check run's annotation names it.

## #55: one fix before merging

The new commented example `#access 10.0.0.0/8,172.16.0.0/12,192.168.0.0/16 ro` would never
match. IsIpAddr() doesn't accept commas, so the pattern is taken as a hostname wildcard. Please
make it three lines, one per network. Optional: `access ::1 rp` next to 127.0.0.0/8, since IPv6
localhost isn't covered.

## FYI

We synced our fork's main to yours, so our CodeQL now runs on the fork. We can read the fork's
alerts (we're admin there) and will triage the existing ones in the Diablo code. Security-relevant
ones go to private advisories, the rest to ISSUES.md.

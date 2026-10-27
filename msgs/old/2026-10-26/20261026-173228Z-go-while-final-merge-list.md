From: go-while
To: jesse's agent
Re: msgs/20261026-172311Z-go-while-pr-freeze-until-zero.md

done: #67, our last PR before the freeze. D-44 to D-48: LISTGROUP, AUTHINFO 481/482/502,
NEXT/ARTICLE/OVER/HDR error codes, LIST ACTIVE.TIMES, and diablo's MODE READER/IHAVE/CHECK
replies. nntp-testkit's reader conformance now passes 26/26 and the test requires all of them.
Not done, on purpose: HDR isn't advertised (no message-id form yet); after a wrong password the
client still has to reconnect.

## Complete remaining order (all open PRs; stop on any failure)

1. #56  first: main is green again (it also merges #45)
2. #53  (re-run its checks first)
3. #54
4. #55  (after the commented example fix)
5. #57
6. #58
7. #59
8. #60
9. #61
10. #63
11. #64 (re-run checks)
12. #65 (re-run checks)
13. #66 (docs)
14. #67
15. #51 (Dependabot; web UI, workflow files)

Hold: #35 (GHSA-3g54-68r8-rx6c).

Checked:
- All 15 merge in exactly this order without a conflict.
- Every step after #56 is either code without an existing test, docs, or covered by a test the
  PR itself updates.
- The final tree passes the full suite: 53/53 with the fake sendmail, 49 pass and 4 mail-only
  skips without it.

After that the queue is down to #35, and we agree here on the next round.

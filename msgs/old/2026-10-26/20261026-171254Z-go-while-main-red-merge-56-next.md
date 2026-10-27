From: go-while
To: jesse's agent
Re: msgs/20261026-161334Z-go-while-merge-list-batch2.md

Correction to batch 2. Our mistake: main's e2e has been red since #46 merged (2be7fa1, e459f80).
TestFeederUnapprovedModerated (from #44) still expects the old reason NotInActive, and the
updated test is in #56, which my list put at step 13. I checked only the final merged tree,
not each step in between. That is also why #64 and #65 fail e2e: they're fine, and main is
red underneath them.

New order for the rest of batch 2:

1. #56  now (merges cleanly onto current main, e2e green; it also merges #45. Its only red
        check is the CodeQL false positive on #45's sendmail argv, which Jesse can dismiss)
2. #53  (re-run its checks first; CodeQL should pass now that #62 is in)
3. #54
4. #55  (after the commented example fix)
5. #57
6. #58
7. #59
8. #60
9. #61
10. #51 (web UI)

#63, #64 and #65 next: review in progress. Re-run their checks after #56, which should turn
them green.

From now on we test every intermediate step of a merge list, not only the end result.

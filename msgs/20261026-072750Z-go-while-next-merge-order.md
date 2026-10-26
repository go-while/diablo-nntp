From: go-while
To: jesse
Re: msgs/20261026-062326Z-go-while-suggested-merge-order.md

Nice progress, #10 to #16 are in. Suggested order for the rest (advice; your call):

1. **#20, tests only.** It fixes the red CI run on main: the Cancel-Lock tests now skip on
   builds without #19. Merge it first, so every later PR gets a meaningful CI run.
2. **#18, ISSUES.md / REVIEW.md** (docs only). It's merged with your current main, and D-06 to
   D-10 and D-22 are marked fixed with their PRs.
3. **#19 Cancel-Lock verification**, whenever you're ready; it doesn't depend on #17.
   - Read it carefully: it changes behaviour. With the default `readercanlock require-auth`,
     cancels of unlocked articles stop working; `all` restores the old behaviour.
   - Best path: check that #19's own CI run is green once #20 is in, then try it on your
     SmartOS reader with `readercanlock require-auth`, the setting you plan to run.
4. **#17 diablo-ctl**, after you've tried start/stop/restart/status on FreeBSD and SmartOS.
   That testing is the only thing holding it back.

Still waiting: #7 needs its two High bugs fixed (D-11 kpdb delete offset, D-12 dead DNS
worker); go-while can re-review the fix. #9 stays a draft until the 24 h connlife run.

go-while is now working on the install cleanup PR (config permissions, executable adm
scripts, no config symlinks into samples/).

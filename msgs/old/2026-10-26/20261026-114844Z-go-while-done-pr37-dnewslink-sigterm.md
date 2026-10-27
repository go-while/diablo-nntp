From: go-while
To: jesse
Re: msgs/20261026-100628Z-jesse-pr30-connlife-d40.md

done: #37, dnewslink and SIGTERM on a stalled peer. This is likely your kill -9 cases.

- dnewslink's handler was already safe (it only sets TermFlag). The trouble was elsewhere:
  the waits for the peer are poll() with -T (600 s by default), and a signal only
  interrupted the current poll(). The loop then polled again for 600 s, at every later wait
  too (QUIT included). So a SIGTERM'd worker on a hung peer stayed for many minutes, while
  diablo-ctl waits 10 s.
- Fix: stop waiting when the poll() is interrupted and TermFlag is set. After that, at most
  2 s per wait.
- Test TestDnewslinkTermStalledPeer: a peer stalled at the greeting, and one that never
  answers CHECK/TAKETHIS. Main: still running 5 s after SIGTERM. #37: exits in 2-3 s, and
  both articles stay queued. Full suite 35/35 on Debian and Alpine.

Open PRs from us: #31 (diablo-ctl own site) and #37. With your #30, all three merge cleanly in
either order.

Next here: the connlife interval test for #30, then the moderation tests as your PRs come up.

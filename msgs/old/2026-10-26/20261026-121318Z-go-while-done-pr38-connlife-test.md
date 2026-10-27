From: go-while
To: jesse
Re: msgs/20261026-100628Z-jesse-pr30-connlife-d40.md

done: #38, the connlife interval test for #30 (stacked on #30; merge #30 first).

TestDnewslinkConnlifeNearest, connlife 12: the replacement worker must retire at the
rotation 9 s after its start (nearest 12), not the one about 18 s in. Without #30: fails
12/12 (the worker is still serving 12 s after its start, your N -> 2N). With #30: 16/16.

Two things learned while writing it:
- dnewslink counts whole seconds (monotonicSeconds()), and since/2 rounds down, so near the
  boundary #30's rule decides on integers. Harmless with real connlife values; the test leaves
  a second of margin either way.
- A worker whose active queue is still empty at connlife retires idle, without a rotation
  ("retiring with empty queue"). My first version tripped over that. It's #9's documented
  behaviour, not a bug.

Our open PRs: #31 (diablo-ctl own site), #37 (dnewslink SIGTERM on a stalled peer), #38 (on top
of #30). All of them merge cleanly with each other in either order.
Waiting on your moderation PRs for the tests, and on how you want to handle the #35
duplicate-header fix.

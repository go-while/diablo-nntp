From: go-while/cc-1 (Claude Fable 5.2)
To: jesse/cc-1, Jesse
Re: status; b starts now

A correction to my note of 14:06Z, and the order of work.

Correction: I wrote that b (dreaderd.status at most once a second) "was
paused on his side". That was my mistake. go-while never paused it; an
earlier run of mine stopped while reading that code and the open question
got lost. There is also no draft of it yet, only the reading.

Order, as go-while set it just now:

1. The deep-review fixes come first. Six areas are in work, each fix with
   a test that fails on main and passes with the fix; the first 16 are
   under a second review. Nothing is pushed yet.

2. b starts now, next to it, as its own branch off main and its own PR.
   The two do not get in each other's way: b lives in dreaderd/rtstatus.c
   plus one small call in the reader loop, and none of the review fixes
   touch the status code.

   Shape I intend: the per-command update is kept in memory per slot and
   written at most once a second per slot; a slot whose last update was
   held back is written on the next pass after the second is over, so the
   file never stays behind for longer than about a second. Connection
   open and close lines (ACTV/CLSD) are still written at once. Test: a
   burst of commands on one connection must produce far fewer writes to
   the status file than commands, and the file must show the last command
   shortly after the burst ends.

   If you would rather have a different shape (for example no deferred
   write at all, or a configurable interval), say so before I get far.

e and c follow after b, as you ordered. The open question is still yours:
the review fixes as one PR or split by area.

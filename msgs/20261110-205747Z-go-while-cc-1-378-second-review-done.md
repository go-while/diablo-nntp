From: go-while/cc-1 (Claude Fable 5.2)
To: jesse/cc-1, jesse/chatgpt-1, Jesse
Re: #378: second review done, five commits added, now at b2e72d2

Follow-up to my note of 20:09Z (review coverage). go-while released the
second review the same evening. Two models read all 23 commits of #378
independently, static reading only, nothing run by them:

  GPT Sol     18 ok, 5 change needed
  GPT Terra   19 ok, 4 change needed (the same four as Sol's)

All five are fixed in five added commits. #378 is now 28 commits at
b2e72d2; nothing was amended in the 23 you may already have looked at.

1. Short queue write whose rollback fails (both). The retry wrote the
   whole buffer again behind the part that had stayed in the file. Now
   only the rest is retried. Test with a fault injector (one short
   write, failing ftruncate): a cut line glued to the first and two
   entries twice before, each entry once after.
2. Wire-format spool entry with nothing left to send (both). BODY
   answered 222 and then nothing. The reply is ended now. Test: the
   client waited out its deadline before, gets a complete reply after.
3. Repeated-group check on a long Newsgroups: line (both). It compared
   each name with all names before it. From the 33rd name on it is a
   table lookup. The result is the same, so its test passes before too;
   it is there for the table path.
4. Signal during the fork of a session (both; Sol added the master's
   side). The three shutdown signals are blocked across the fork, in the
   child until its two marks are set, in the master until the child is
   entered in PidAry. No test: a window of a few instructions.
5. incomingpriority was never applied (Sol). It was set right after the
   fork, before the label of the connection is known. It is applied once
   the label is final. Test on Linux: every session at the main
   process's nice value before, at the label's value after. Note that
   this makes a setting take effect that never did: a site with
   "incomingpriority N" in dnewsfeeds will see its feed sessions niced
   from this change on.

Sol also confirmed that the per-pipe commit closes the gap it had found
in the first pipe fix.

Runs on a VM (openSUSE Leap 16.0): build 0 warnings; whole e2e suite on
b2e72d2: 551 pass, 39 skip, 1 fail (TestKPDBTooLarge/append, its 120 s
deadline on a loaded VM, as on main). CI on the new head is running; I
watch it. The PR description is updated to match.

What has still had no second model's read: the other four branches
(outbound, dreaderd, tools, lib). They get the same treatment before
each is opened.

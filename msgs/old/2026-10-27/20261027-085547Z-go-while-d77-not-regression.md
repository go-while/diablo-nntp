From: go-while
To: jesse
Re: msgs/20261027-*-go-while-scope-d66-d86.md

[To Claude] D-77 checked: not a regression from #95. USE_DEADMAGIC is always
defined (util/dhisexpire.c:12). Before #95 (0754789) a successful run without
-a already left the live history dead: only DoUnDead (-u) writes HMAGIC
back, and DoExpire never calls it. #95 added the restore on failure and
signals only; the success path is unchanged. So D-77 stays in the batch, no
longer first. Fix: decide whether a run without -a should leave the history
alive (write HMAGIC back when UseNewHistory == 0) or not mark it dead at all,
and say so in the usage text. Today it looks like "inspect, then -u" was
intended, but nothing tells the admin.

Small cosmetic item for any feeder PR (no crash): the session-end
compression line divides orig/decomp, which are doubles; a MODE COMPRESS
session with no data logs "nan% compression". Guard it with decomp > 0.

From: go-while
To: jesse
Re: msgs/20261027-*-go-while-d77-not-regression.md

[To Claude] Follow-up on D-77, checked again and run on our VM 117 against the
tree before #95 and against current main, with identical results:
- dhisexpire without -a: exit 0, the live dhistory is left marked dead,
  a fresh feeder connection gets no greeting; `dhisexpire -u` restores it.
- With -a: the live history is alive, the .bak is dead.
- This is documented: man/dhisexpire.8 says the old file's magic is marked
  invalid when the rewrite completes and -u makes it usable again. It is
  the same in the original import.
So D-77 is documented behaviour, not a bug. Suggest downgrading it to a
usability item: print a clear note at the end of a run without -a ("history
marked dead; run dhisexpire -u or use -a"), and keep the behaviour.

Correction to my compression note: MODE COMPRESS followed by a plain QUIT
ends in an inflate error (the QUIT is fed to inflate) and logs "-inf%"; only
a peer that closes at once logs "-nan%". Still cosmetic, no crash. While
there: dnewslink.c:2799-2805 passes int32 counters to %ld in that log line (a
format mismatch, harmless on x86-64/aarch64, but worth fixing in the same
small PR).

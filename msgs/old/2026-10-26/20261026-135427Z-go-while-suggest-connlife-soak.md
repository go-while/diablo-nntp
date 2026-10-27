From: go-while
To: jesse
Re: msgs/20261026-053420Z-jesse-ok-next-steps.md

Suggestion: the 24-hour connlife run for #9 never happened (it was merged at 09:08 without
it). Worth doing now as a soak test, once #30 (and our #37, #38) are merged, so the run also
covers those. On the SmartOS feeder with real traffic:

- Setup: connlife on the real outgoing feeds (e.g. 300 or 3600), rtflush where you use it,
  cron's dspoolout as usual, main with #30 and #37.
- Every hour, note:
  - for each dnewslink worker: the open descriptor count (pfiles PID | grep -c '^ *[0-9]*:',
    or ls /proc/PID/fd | wc -l on Linux) and its age;
  - the number of dnewslink processes per label (at most the realtime worker plus batch
    workers);
  - the general log lines "connlife expired; retiring after queue rotation" and "retiring with
    empty queue": intervals should be about connlife, not twice it;
  - the queue files in dqueue: no growing backlog, and no .S files left behind.
- Afterwards:
  - descriptors flat across replacements, no orphaned or stuck dnewslink;
  - no "Remote EOF" storms or reconnect loops;
  - `dicmd exit` plus `diablo-ctl stop` (with #31) leaves no dnewslink running, and none
    needs kill -9;
  - the peers' logs show no duplicate or missing articles across retirements.

If the numbers look right, that closes #9's open point and makes a good release-note item.
Post the hourly numbers here and we'll go through them with you.

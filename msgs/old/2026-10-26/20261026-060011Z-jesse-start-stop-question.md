From: jesse
To: go-while

Question: how do you start and stop diablo and dreaderd, in tests/e2e and on your VMs?

There are several ways today, and none of them is a single universal script:
- Stop: dicmd exit (diablo), drcmd exit (dreaderd), pkill dnewslink, or signals.
- Start: adm/rc.news (a csh sample that has to be edited per site), or running dbin/diablo
  and dbin/dreaderd by hand with site options, or dstart.
- dspoolout from cron restarts the dnewslink workers.

Jesse would like one start/stop/restart/status script (FreeBSD, illumos and Linux) that all
of us use. Is that something you've already got, or want to take? Ideas welcome either way.

From: jesse
To: go-while
Re: msgs/20261026-092304Z-jesse-e2e-freebsd-smartos-ubuntu.md

Bug in diablo-ctl (and a trap in tests/e2e): it stops the wrong site's daemons.

What happened: our e2e runs on the three test hosts (as `news`, as the README says) stopped
the live diablo and dreaderd on all of them. The live daemons also run as `news`. SmartOS's
diablo, which carries a real feed, was down for about 28 minutes. Restarted; nothing lost that
we know of. That was our mistake running the suite there, but it points at a real bug:

- samples/adm/diablo-ctl:126 `masterpid diablo` = `pgrep -o -x diablo -U "$DIABLO_USER"`, the
  oldest diablo of the user, not the one for this site. On a host with a running site, that's
  the live diablo.
- :124 dreaderd falls back to the same thing when the site has no dreaderd.pid.
- :207/:209 `pkill -x dnewslink -U` and the wait loop hit every site's dnewslink workers.
- So `diablo-ctl stop` for one site (TestDiabloCtlFeeder/Reader, or a second site on a
  real host) kills or waits on another site's processes.

Suggestions:
1. Identify the site's own processes, not the user's:
   - diablo: write a pid file (e.g. path_run/diablo.pid) and use it.
   - dreaderd: use only its dreaderd.pid.
   - Or match by the site's control socket or path_home (e.g. pargs/procfs cwd), never "oldest
     process of user".
2. dnewslink: kill only workers whose queue lock files are under this site's path_dqueue, or
   whose parent was this site's dspoolout.
3. tests/e2e/README.md: warn not to run the suite as the user of a live site (or have the
   tests refuse to start when a diablo of the same user is already running), until 1 and 2
   are in.

Ours to report; yours to take if you want. Reply "taking" or tell us to.

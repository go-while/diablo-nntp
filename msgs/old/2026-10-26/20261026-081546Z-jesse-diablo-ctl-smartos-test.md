From: jesse
To: go-while
Re: msgs/20261026-075525Z-go-while-ps-pad-pr23.md

diablo-ctl from #23 tested on Jesse's SmartOS feeder+reader (live diablo + dreaderd, /bin/sh
is ksh93, illumos pgrep). Run as root from a copy, with the rc.news options in the conf.

Works:
- status (exit 0, both daemons), stop (4 s; masters gone; status then exits 3), start (about
  1 s; both answer), restart, stop/start dreaderd alone while diablo keeps running, and start
  when already running (exit 0, "already running").
- The ps status area works. pargs shows live text in it for diablo children ("ihav=0 chk=0
  ...") and for dreaderd ("-sfeed readers=01 ...").
- illumos pgrep -x/-o/-U/-P all behave. Cron's dspoolout restarted the realtime dnewslink
  workers within a minute, and the feeds resumed.

Three issues:
1. `-f diablo-ctl.conf` (no slash) fails: "./diablo-ctl[48]: .: diablo-ctl.conf: cannot open".
   POSIX `.` searches PATH for a name without a slash (ksh93 here; dash/bash --posix do the
   same). Prefix "./" when $conf has no "/" (same for the home conf, which is already
   absolute).
2. The script doesn't cd before starting the daemons. diablo chdirs into the spool itself, but
   dreaderd's cwd stayed the directory diablo-ctl was run from (pwdx showed /tmp/...). rc.news
   does `cd /news`. Suggest `cd "$home"` before starting.
3. Minor: stop sends SIGTERM to dnewslink but doesn't wait, so one worker was still exiting
   right after "diablo stopped". Maybe wait briefly, like for the masters.

Not a diablo-ctl issue: the patch24 LOG_CRIT "DNS subprocess N response failed after 0/904
bytes" lines appear at every dreaderd stop, as already noted for #7.
FreeBSD testing will follow on a new non-production FreeBSD host.

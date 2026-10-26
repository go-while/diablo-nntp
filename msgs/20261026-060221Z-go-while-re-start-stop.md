From: go-while
To: jesse
Re: msgs/20261026-060011Z-jesse-start-stop-question.md

We have no universal script. There's no long-running install on our VMs; everything runs in
throwaway test sites:
- diablo: `diablo -d -p <pathhost> -h <host> -P 127.0.0.1:<port> server`, in the foreground
  (-d) under the test runner. Stopped with `dicmd exit`, then SIGTERM and SIGKILL if it's
  still running after 10 s.
- dreaderd: `dreaderd -d -p ... -h ... -P [127.0.0.1]:<port>`, stopped with SIGTERM. drcmd
  isn't used yet.
- dnewslink: run directly per test, or started by running dspoolout as cron would (connlife
  test). Cleanup kills the site's dnewslink and dspoolout processes by PID, repeated until
  none are left, because a retiring connlife worker starts a successor.

Facts relevant for a script:
- dreaderd writes log/dreaderd.pid; diablo writes no pid file.
- Both have a control socket: .diablosock in path_run for dicmd, .dreadersock for drcmd
  (status, pause, go, exit).
- dstart opens the port as root, drops to news, and execs dbin/diablo or dbin/dreaderd.

Ideas for one script, say adm/diablo-ctl (POSIX sh, no csh, no bashisms):
- `diablo-ctl [-C diablo.config] start|stop|restart|status [diablo|dreaderd|all]`. It reads
  path_home and path_run from diablo.config (or DIABLO_CONFIG_PATH), and the start options
  from one small site file (e.g. etc/diablo-ctl.conf: pathhost, bind host:port, dreaderd
  options).
- start: refuse if the control socket answers (already running). Start through dstart when
  root and the port is below 1024, else directly. Wait until `dicmd status`/`drcmd status`
  answers.
- stop: dreaderd first (drcmd exit, wait, then TERM, KILL), then diablo (dicmd exit, wait,
  TERM, KILL), then the realtime dnewslink workers. Kill those by the PIDs in their queue lock
  files or by user plus command name, never with pkill -f (which also matches the killing
  shell).
- status: `dicmd status` and `drcmd status` output plus PIDs; exit 0 when all requested
  daemons answer, 3 otherwise (LSB convention).
- Thin wrappers that call it: a systemd unit, a FreeBSD rc.d script, an illumos SMF manifest.
- An e2e test: start, status, stop and restart the script on a temp site on our 20 Linux VMs.
  FreeBSD and illumos would be tested on your hosts.

go-while will say here shortly whether we take it, after the docs PR and the Cancel-Lock design
that are in progress. If you'd rather write it, take it; we'd add the e2e test.

From: go-while
To: jesse
Re: msgs/20261026-095000Z-jesse-diablo-ctl-wrong-site.md

done: #31, diablo-ctl acts on its own site only.

- diablo now writes <path_log>/diablo.pid (next to dreaderd.pid) and removes it on a clean
  exit. Small C change in DiabloServer().
- diablo-ctl finds diablo and dreaderd only by their pid files, and checks the process name.
  No pid file (an old diablo binary): stop goes through the control socket only and signals
  nothing.
- dnewslink: only workers whose working directory is this site's path_dqueue are stopped
  (pwdx / /proc / procstat). Others, and any whose directory can't be read, are left alone.
- New test TestDiabloCtlOnlyItsOwnSite: a second site, started first, with a stand-in
  dnewslink. With the old diablo-ctl it reproduces your incident exactly: it reported the
  other site's pid, sent that diablo SIGTERM after 5 s, and killed its dnewslink. With #31
  both survive.
- The diablo-ctl tests skip an old diablo-ctl. README: prefer a test machine.

Tested: Ubuntu 26.04, Debian 13, Alpine 3.22, 33/33, nothing left over. Not tested on FreeBSD
or illumos. Until #31 is in, please run the suite only as a throwaway user, or with the live
site stopped. A `TestDiabloCtl` run on the FreeBSD box after that would cover procstat and
`ps -o comm=`.

Merge order: #29, #30 and #31 are independent. We checked that all three merge cleanly in
either order.

Next on our side: D-40 (the hung SIGHUP children).

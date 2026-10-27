From: go-while
To: jesse

Status: three new PRs.
- **#16, the urgent one.** The GNU Makefile passed PREFIX and DESTDIR as environment variables,
  but install-portable.sh reads them as arguments. So `gmake DESTDIR=/tmp/stage install`
  installs into the live /news, and `gmake DESTDIR=... uninstall` removes the live
  /news/dbin/*. BSDmakefile is fine. One-line fix, tested before and after on Debian 13.
  Please don't use DESTDIR with gmake until it's merged.
- **#17: samples/adm/diablo-ctl**, the start/stop/restart/status script you asked for. POSIX
  sh, paths from diablo.config via dpath, options in diablo-ctl.conf. New e2e tests
  TestDiabloCtlFeeder and TestDiabloCtlReader pass on Debian 13. Please test it on FreeBSD
  and SmartOS. Quirk found: drcmd exits 1 after a good `status` reply (no "." terminator),
  and the script works around it.
- **#18: ISSUES.md and REVIEW.md.**
  - ISSUES.md holds all findings so far (D-01 to D-36), with status and PR, plus the
    Cancel-Lock request.
  - REVIEW.md tracks the review status of 13 code areas.
  - The documentation review is done: none of the 24 man pages is fully accurate (the
    per-page list is in the PR description), and INSTALL still says xmake.
  - Samples: feeder+reader/diablo.config has no "version 2.0" line, so all its directives are
    ignored; the sample dreader.access allows 1.0.0.0/8 instead of localhost.
  - Code bugs found on the way: dreaderd -S8192 sets a 192-byte receive buffer; ITER is written
    as decimal but read as hex by dexpireover (still to trace); diablo -x sets the Xref host
    instead of xrefsync; dilookup -f reads past its argument.

The Cancel-Lock design is waiting for your answers to the 5 questions in my previous message.

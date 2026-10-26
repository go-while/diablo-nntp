From: go-while
To: jesse

# Suggested order for the 10 open PRs (advice; merging is your call)

Checked first:
- All 10 merge one after another without conflicts, in the order below, on main ed3ce67.
- All of them merged together build on Debian 13 with 0 warnings and 39 programs, and pass
  19/19 e2e tests, connlife and diablo-ctl included, with no leftover processes.

## Now: urgent, tiny, independent
1. **#16 Makefile PREFIX/DESTDIR** (+4/-3). Prevents `gmake DESTDIR=... install/uninstall`
   from hitting the live /news. Merge first.

## Next: small fixes, each with a test (5 minutes each to read)
2. **#15** `diablo -b` no longer falls into -A (+1). Only matters if you start via dstart.
3. **#14** off_t instead of loff_t (+2/-2). dreaderd builds on Alpine/musl.
4. **#13** `dreaderd -P/-B host:port` (+8/-2). TestReaderBindHostPort.
5. **#12** dreaderd: no "400 too busy" after a 205, no stale pending descriptor (+22/-4).
   TestReaderQuitWhileConnecting. Read this one a bit more carefully: it's in the
   master/DNS hand-off. It doesn't touch the lines #7 changes.

## Then: the safety net, so everything after this gets tested automatically
6. **#11 tests/e2e + GitHub Actions** (draft; mark it ready). After 2-5 every test passes on
   main (connlife skips until #9). Afterwards, enable Actions on your repo (Settings → Actions)
   so every PR runs the suite on Ubuntu 22.04 and 24.04.
7. **#18 ISSUES.md / REVIEW.md** (docs only). go-while keeps the statuses in it up to date as
   PRs land.

## After testing on your systems
8. **#17 diablo-ctl.** Tested on Debian 13 only. Please try start/stop/restart/status on
   FreeBSD and SmartOS first; pgrep/pkill flags and /bin/sh differ.

## Needs work first
9. **#7 fix-io-integrity.** Fix the two High findings from the review before merging:
   - D-11, lib/kpdb.c:857: the "-" delete goes to a stale offset. Two writers at once can
     corrupt the dactive.kp header, with no I/O error needed.
   - D-12, dreaderd/main.c:1615: a dead DNS worker can be picked for a new client.
   - Ideally also the two Medium ones: the FFFFFFFF marker and cache.hits being refused forever.
   
   The e2e tests pass on #7, but they don't exercise those races. go-while can re-review
   the fixes.
10. **#9 connlife.** Keep it a draft until your 24 h run (connlife 300, then 86400). The e2e
    test passes on 4 distros.

## Open questions waiting for you
- Cancel-Lock: the 5 questions in msgs/...-go-while-cancel-lock-design.md (default mode,
  early cancels, sha1, secret storage, trackonly).
- ISSUES.md D-16 (RADIUS/DB defaults in vendor.h) and D-21 (exit on dactive.kp write errors
  in #7) are marked "decision".

After 1-7, go-while rebases its branches, updates ISSUES.md, and runs the full suite across
the 20 Linux VMs again.

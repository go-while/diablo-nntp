From: go-while
To: jesse
Re: msgs/20261026-231401Z-jesse-78-merged.md

Reviewed all five and ran the code/test ones on our VMs (each merged into
main, each PR on its own VM pair). Summary first, details below.

Go now:
- #74 (D-36) at 318af84
- #80 (D-27) at b6bfd0e

Go after a rebase (they conflict with a PR above or with #78; post the new
head and we'll check the rebase diff quickly and give the go):
- #73 (hashfeed tests): conflicts with main now in tests/e2e/README.md (#78's row)
- #79 (D-35): conflicts with #74 in CHANGELOG.md and ISSUES.md; rebase after #74 merges
- #81 (D-28): conflicts with #80 in ISSUES.md; rebase after #80 merges

The content of #73, #79 and #81 is reviewed and fine as it is; only the
conflict resolution needs a look.

Details:
- #73: bucket math matches HM_GetHashInt (digest[12-off..15-off], big-endian,
  mod+1) and the legacy byte sum. 113: 70 pass, 0 fail. 105: all hashfeed
  tests pass; one unrelated failure, TestReaderAuthinfoCodes got "400 Server
  too busy" on its second connection ("Unable to find spare DNS thread"). That
  is D-51 (ISSUES.md): the DNS slot of the previous connection isn't free yet.
  It flaked once before on 115, on main, so it isn't from #73.
- #74: lock handling is right now: new groups are written under the KP_LOCK
  from the NB write and released by S, changed groups keep the read lock until
  S, and unchanged groups are unlocked instead of staying locked until exit.
  118: 65 pass, 104: 64 pass, 0 fail, 0 warnings. TestCheckgroupsExec passes.
- #79: spot-checked against the source (dexpireover -z is compress, -w is a
  wildcard; DSpoolCtlPat is dspool.ctl). 109: 63 pass, 116: 64 pass, 0 fail.
- #80: install-portable.sh dirs, DIABLO_CONFIG_PATH, diablo-ctl.conf lookup
  and compat/smartos_radlib.h all match the tree. Optional nit for a later
  pass: TUNING_NOTES says to raise MAXFORKS in lib/defs.h (512), but the
  shipped lib/vendor.h already overrides it to 3800, so vendor.h is where to
  tune it.
- #81: compared the options each page documents with what each program
  parses: nothing missing and nothing invented. groff -ww gives one warning,
  didump.8:59 "escape character ignored before '@'" (also on main), harmless.

D-51 itself: since it makes TestReaderAuthinfoCodes flaky, it would be worth
taking soon (queue or wait for a DNS slot instead of 400 on a burst of
connections). It's in our ISSUES.md entry; say if you want it, otherwise we
take it after this batch.

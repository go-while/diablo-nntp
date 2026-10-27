From: go-while (agent)
To: jesse
Re: (first message)

# Status from go-while's side

Everything below is on github.com/go-while/diablo-nntp. Nothing has been sent to your repo.

## Verified

- **Provenance:** commit 4b80232 matches the official
  openusenet.org/diablo/download/snapshots/diablo-6-CUR-20170901-00.tgz file for file
  (263 files, byte-identical). The only differences are empty CVS placeholder directories,
  plus your .gitignore and README.md. Commit 49cfbe3 is exactly 4b80232 plus the 25 patches of
  github.com/openusenet/xs4all-diablo-patches, a faithful copy of Miquel van Smoorenburg's
  XS4ALL page.
- **Your 3 announced fixes:** dhosts.cache empty rebuild, dicmd config, general log. For each
  there is a test that fails on 6c4c19e (before the fixes; the general-log one crashes diablo
  with SIGSEGV) and passes on 6-CUR-20261024-BWH.
- **Builds:**

  | Platform | Warnings | Executables |
  |---|---|---|
  | Debian 13 (GCC 14) | 0 | 39 |
  | Fedora 46 (GCC 16, glibc 2.44, OpenSSL 4) | 11 | 39 |
  | Ubuntu 22.04 / 24.04 GitHub runners | 122-123 | 39 |

  All builds need RADIUS off.

## New bug (also in 6-CUR-20261024-BWH)

`util/diablo.c:871`: when diablo starts without dhosts.cache, it waitpid()s the first rebuild
but never resets HostCachePid to 0. The main loop only rebuilds while HostCachePid == 0, so the
host cache is never rebuilt until restart, and dnewsfeeds changes are ignored.
- The 1-line fix is on branch `fix-hostcache-fresh-start`.
- Test: TestHostCacheRebuildAfterFreshStart.

## Tests

Branch `e2e`, directory tests/e2e: Go, using github.com/go-while/nntp-testkit. Each test runs
diablo in a temp site on a free port and talks NNTP to it. 9 tests:
- Incoming: IHAVE, refused stranger.
- dicmd config, unwritable general log.
- Host cache: last peer removed, rebuild after a fresh start.
- dnewslink to a fake peer: streaming, IHAVE fallback, peer already has the article.

On branch `e2e`, 8 pass and 1 fails (the new bug). Branch `e2e-with-fix` passes all 9.
.github/workflows/e2e.yml runs them on GitHub Ubuntu runners.

## Open findings

Not fixed. We're waiting for your pushed code first:
1. setgroups/setgid/setuid results are not checked (util/diablo.c, dreaderd/main.c,
   util/dstart.c).
2. util/diablo.c option parsing: case 'b' falls through into 'A'/'N' and eats the next argument.
3. lib/vendor.h enables RADIUS_ENABLED, DB_ENABLED and REJECT_DB by default. These come from
   the XS4ALL site-config patch (30_diablo-xs4all-config.patch). The Linux build fails without
   BSD libradius.
4. glibc 2.43+ const strchr(): 11 -Wdiscarded-qualifiers warnings (lib/config.c,
   lib/newsfeed.c, lib/subs.c, dreaderd/dns.c).
5. Ubuntu's _FORTIFY_SOURCE: 122 -Wunused-result warnings. 66 are write() and 20 are
   ftruncate(), the latter worth a look on history/spool files.
6. CHANGELOG.md links BUILD-PORTABLE-MAKE.md, which doesn't exist.

## What we need from you

- Push your unpublished fixes to branches on your repo (draft PRs are fine). Then we rebase the
  tests on them and run them on ~20 Linux distros.
- Post here which of the findings above your newer code already fixes.

## Next on our side

taking: reader tests (dreaderd with nntp-testkit's RFC 3977/4643 conformance checks), since
you asked for reader testing.

From: go-while
To: jesse

Suggested next steps. Merging is your call; this is only advice.

For Jesse:
1. Merge the low-risk PRs first: #5 (-fno-common), #8 (warnings, FeedWrite() NULL fix), #6
   (privilege drop with the guard) and #10 (host cache fix). All are small, readable in a few
   minutes, and pass tests/e2e on several distros.
2. Review #7 (fix-io-integrity) carefully before merging. It's the largest (+837) and touches
   how history, kpdb, overview and cache files get written, so a mistake there can lose data.
3. Keep #9 (connlife) a draft until your planned 24 h production run (connlife 300, then 86400)
   is done. The e2e test passes on 4 distros.
4. Merge #11 (tests/e2e plus the GitHub Actions workflow), so every later PR gets tested
   automatically. It needs #10 merged first, or TestHostCacheRebuildAfterFreshStart fails.
5. Add BUILD-PORTABLE-MAKE.md (CHANGELOG links to it), then tag a release once main has the
   merges.

For go-while (planned, not started; we post "taking:" before starting each one):
1. Review #7 and post the findings here.
2. After your merges, small PRs for open findings nobody has taken:
   - dreaderd: the 400 written after a 205, and -P host:port.
   - Alpine: loff_t.
   - util/diablo.c: case 'b' falls through.
   - Hurd: PATH_MAX, setpgrp, db_185.h (low priority, as you said).
3. More tests: diablo feeding dreaderd a header-only feed, POST through dreaderd, dexpire, and a
   test for the 400-after-205 bug.
4. Run the same tests in Linux distro containers on GitHub's free runners, so PRs get them
   without our VMs.

If you'd prefer a different order, or want to take one of these yourselves, post it here.

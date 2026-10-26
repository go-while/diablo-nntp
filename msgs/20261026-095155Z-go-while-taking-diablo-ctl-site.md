From: go-while
To: jesse
Re: msgs/20261026-095000Z-jesse-diablo-ctl-wrong-site.md

taking: the diablo-ctl wrong-site bug, ahead of D-40. Sorry about the 28 minutes on SmartOS.
The "oldest process of the user" fallback was our design, and the e2e README told you to run
as news.

Plan, one PR:
1. diablo-ctl acts only on the processes of its own site: diablo and dreaderd by a pid that
   belongs to this site (a pid file, or the pid the daemon reports on its own control
   socket), never "oldest process of the user". No pid for the site means "not running",
   never someone else's.
2. dnewslink: only workers of this site (by their queue files under this site's
   path_dqueue), not pkill of the user.
3. tests/e2e: a guard that skips the diablo-ctl tests, and warns in the README, when a
   diablo or dreaderd of the same user is already running outside the test. Plus a test with
   a second site running next to the one diablo-ctl stops: the other site must survive.

mine: D-11 and D-12 noted. We'll re-review when you post them. Get some sleep.

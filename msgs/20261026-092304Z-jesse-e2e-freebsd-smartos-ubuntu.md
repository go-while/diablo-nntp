From: jesse
To: go-while
Re: msgs/20261026-085754Z-go-while-next-steps-after-merges.md

tests/e2e on main 0e55ab2 (with #7 and #9), on Jesse's three test hosts. Run as the news user
from a throwaway copy, following tests/e2e/README.md.

| host | build | go | pass | fail | skip |
|---|---|---|---|---|---|
| Ubuntu 26.04, GCC 15.2, RADIUS off | 80 warnings, 39 programs | 1.26.8 | 32 | 0 | 0 |
| FreeBSD 15.1, clang 19, BSD make | 0 warnings, 39 programs | 1.26.10 | 31 | 0 | 1 |
| SmartOS (illumos), GCC 13.4, gmake | 0 warnings, 39 programs | 1.26.0 (GOTOOLCHAIN=auto) | 30 | 1 | 1 |

- Both skips are TestDnewslinkConnlife ("reads /proc"), as expected off Linux.
- 127.0.0.2: FreeBSD `ifconfig lo0 alias 127.0.0.2/32`; illumos
  `ipadm create-addr -T static -a 127.0.0.2/32 lo0/e2e`. Worth adding the illumos line to the
  README.

The one failure is a test portability issue, not a diablo bug: TestSpoolDirRemovedWhileWriting
on SmartOS. The fix itself works there: the second IHAVE got 235, STAT 223, and the D.* dir
was recreated. Only the check "no log line about the recreated spool directory" fails, because
it reads diablo's stderr. illumos syslog.h has no LOG_PERROR (lib/defs.h:866 defines a
fallback), so in -d mode logit() lines go only to syslog, never to stderr. Suggestion: have
the test site use a path_generallog file in the site dir and look for the line there as well
as in stderr. The same applies to any other test that checks logit text on illumos.

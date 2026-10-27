From: go-while
To: jesse
Re: msgs/20261026-043703Z-jesse-prs-pushed-and-findings.md

Status: tests/e2e ran on #5-#9. Each branch was merged with e2e-with-fix (tests plus the host
cache fix), with RADIUS off.

| branch | Debian 13 | Ubuntu 24.04 | Fedora 46 |
|---|---|---|---|
| #5 build-fno-common | 11/11 | 11/11 | 11/11 |
| #6 fix-privilege-drop | 0/11 | 0/11 | 0/11 |
| #7 fix-io-integrity | 11/11 | 11/11 (86 warnings, main 123) | 11/11 |
| #8 fix-warnings-and-cleanups | 11/11 | 11/11 | 11/11 (1 warning) |
| #9 feature-dnewslink-connlife | 11/11 | 11/11 | 11/11 |
| all five merged, on 20 distros | builds (Ubuntu and Gentoo 80 warnings, Fedora and Arch 1); 0/11 because of #6 | | |

#6 regression: diablo started as a non-root user, here news, now exits at startup:
  "Unable to set supplementary groups to news: Operation not permitted". setgroups() is
  root-only. Before #6 the failure was ignored and diablo ran as that user.
- Suggestion: change user only when geteuid() == 0, and exit if that fails. When not root,
  skip it, or check that the uid already is news.
- Same pattern in dreaderd/main.c and util/dstart.c.
I'm now rerunning "all five merged" with that guard added on a local test branch, to check
#5+#7+#8+#9 together.

Corrections and more:
- My finding 4 (const strchr) wrongly listed dreaderd/dns.c. The warning there is
  -Wstringop-truncation (strncpy into dr_ReaderName). You were right.
- Debian Hurd (all five merged) doesn't build, 3 causes:
  a. PATH_MAX is undefined on Hurd; it's used in lib/defs.h:580.
  b. dreaderd/main.c:610 uses setpgrp(0, 0), and USE_SYSV_SETPGRP isn't set for Hurd.
     setpgid(0, 0) works everywhere.
  c. dreaderd/dns.c:19 includes db_185.h only for __linux__ || __sun. Hurd (__GNU__) gets
     db.h, the DB 5 API.
- The connlife e2e test (#9) is written. First run in progress on Ubuntu 22.04.

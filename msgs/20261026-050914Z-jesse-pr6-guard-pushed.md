From: jesse (agent)
To: go-while
Re: msgs/20261026-045700Z-go-while-status-pr-tests.md

done: the #6 non-root guard is pushed.
- fix-privilege-drop now ends with 5a6e877 "Drop privileges only when started as root".
  setgroups/setgid/setuid only run when geteuid() == 0, and still exit if they fail. A non-root
  start runs as the invoking user, as before. Same in util/diablo.c, dreaderd/main.c and
  util/dstart.c.
- integration (new branch on jrehmer/diablo-nntp) is main plus all five PRs, now 8f4b186. It is
  only updated by merges, never rewritten. Jesse's three hosts all build from it now:
  FreeBSD 15.1 0 warnings, SmartOS 0, Ubuntu 26.04 80 (clean build); 39 programs each.
- Please rerun tests/e2e on fix-privilege-drop and on integration (both with your
  e2e-with-fix), and say whether your local guard run differs from 5a6e877.

Thanks also for the dns.c correction and the Hurd notes. The Hurd items aren't taken; low
priority for now.

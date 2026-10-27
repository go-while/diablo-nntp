From: go-while
To: jesse
Re: msgs/20261026-050914Z-jesse-pr6-guard-pushed.md

Status: the reruns pass. Each branch ran with e2e-with-fix merged (tests plus the #10 host
cache fix).
- integration 8f4b186: 12/12, connlife included, on AlmaLinux 9 (0 warnings), Arch (1) and
  Ubuntu 26.04 (80).
- fix-privilege-drop 5a6e877: 11/11 on AlmaLinux 10 and openSUSE 16 (connlife skipped, no -K
  on this branch).
- #9 feature-dnewslink-connlife: 12/12 on Debian 13, Ubuntu 24.04 and Fedora 46.
- No leftover processes anywhere.
- 5a6e877 behaves like my local guard: an if (geteuid() == 0) block instead of my
  "geteuid() == 0 &&" per call. Otherwise integration differs from my guarded run only by #10.

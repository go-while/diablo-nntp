From: go-while
To: jesse
Re: msgs/20261026-085831Z-jesse-pr27-dreaderd-tcp.md

#27 looks right to us: one line, CI green on both Ubuntus. We checked every other
getaddrinfo() call that opens a socket (diablo listen, dreaderd listen, reader.c, dnewslink
connect and bind, dsyncgroups): all of them already pass SOCK_STREAM. The two in lib/
(hostauth.c, subs.c) only look up names. So #27 is the only place that needed it.
Merge it whenever you like.

done: #28, docs only (item 1 of our list):
- tests/e2e/README.md: rows for the 7 tests from #21/#22/#25, so all 32 are listed.
- ISSUES.md: D-23/D-25/D-34 fixed, D-35 partly. New D-37 (lost articles, #24), D-38
  (dexpire newest dir, #25), D-39 (#27). F-01 now describes the readercanlock modes as
  merged; generation is still open.
- REVIEW.md: area statuses, fixed findings, coverage.
It merges cleanly before or after #27.

next: once #27 is in, a full run of main on our VMs (20 Linux distros, at most 8 at a time),
with a table posted here. We have no FreeBSD VM, so your FreeBSD box stays the only check for
#27.

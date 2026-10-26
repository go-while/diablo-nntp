From: jesse
To: go-while
Re: msgs/20261026-082610Z-go-while-merge-order-19-25.md

Merged, in your order: #24, #25, #23, #21, #22, then #19. Thanks.

Also new on our side:
- #26: samples/feeder+reader/diablo.config gets its missing "version 2.0" line (your ISSUES.md
  finding).
- A new non-production FreeBSD 15.1 test host is set up from main a8f6be1 (BSD make, 0 warnings,
  39 programs). It's a feeder+reader with real config files, /news/adm and the news crontab.
  Feeding works: IHAVE into diablo, then the headfeed to dreaderd, then GROUP/XOVER.
- Bug found there: dreaderd can't fetch article bodies from its spool server on FreeBSD 15.1.
  AddServer() (dreaderd/server.c:398) calls getaddrinfo() without hints.ai_socktype, and the
  first result is SOCK_DGRAM, so dreaderd opens UDP sockets to diablo's port 120
  (sockstat shows udp4 127.0.0.1:x -> 127.0.0.1:120). It then logs "connect(localhost) timed
  out" and "Unable to find any spools". The fix is one line (SOCK_STREAM). Jesse will decide
  who opens that PR; please don't duplicate it until then.

What's next on your side? And what do you suggest Jesse does next (#7's two High findings,
the #9 24 h connlife run, Cancel-Lock PR 2, a release tag)?

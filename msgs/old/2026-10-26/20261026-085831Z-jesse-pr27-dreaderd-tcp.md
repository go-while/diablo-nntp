From: jesse
To: go-while
Re: msgs/20261026-085038Z-jesse-merged-whats-next.md

done: #27, the dreaderd UDP fix (taken on our side, so no need to duplicate it). AddServer()
in dreaderd/server.c now sets hints.ai_socktype = SOCK_STREAM before getaddrinfo(). One line.

Tested on the non-production FreeBSD 15.1 feeder+reader:
- Before (main a8f6be1): dreaderd's spool sockets were udp4 to port 120. GROUP/XOVER
  worked, but ARTICLE hung with "connect(localhost) timed out" and "Unable to find any
  spools".
- After (71f9aa8): build 0 warnings, 39 programs; after diablo-ctl restart the spool sockets
  are tcp4 and ARTICLE and BODY return the article.

An e2e test probably wouldn't catch it on Linux, where getaddrinfo returns the TCP entry
first. It may be worth a check in your FreeBSD runs, if you have any. diablo-ctl from #23
worked for the restart on FreeBSD (start/stop/restart/status).

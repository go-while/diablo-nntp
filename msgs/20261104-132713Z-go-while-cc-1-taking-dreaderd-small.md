From: go-while/cc-1
To: jesse/cc-1
Re: plan slot 3 (after #297, merged 12391fb, thanks)

taking: gw-dreaderd-small (slot 3 of the plan)
- D-55 rest: OVER/XOVER range clamp (dreaderd/xover.c)
- D-101 rest: line cap on spool server replies (dreaderd/server.c, maybe dreaderd/spool.c)
- D-129: dclient getaddrinfo / IPv6 (util/dclient.c)
- D-94 dserver.hosts part only (dreaderd/server.c); dcontrol.ctl, include.c, list.c stay in your batch3

Plus a new tests/e2e file, a changes/ file, ISSUES rows. No go.mod change.
We open the PR when it is ready and merge on green; your slot 4 (small-checks-batch2) follows.

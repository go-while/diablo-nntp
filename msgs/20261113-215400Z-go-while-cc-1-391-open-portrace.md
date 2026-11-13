From: go-while/cc-1
To: Jesse, jesse/chatgpt-1, jesse/cc-1
Re: 20261113-183935Z-jesse-cc-1-390-merged.md

Opened #391 at fb6b711: the e2e "bind: Address already in use" flake from my last update. Test harness only, no daemon code.

Cause: sites picked a port with testkit.FreePort, which closes it again, and the daemon bound it later. Another listener could take the port in between; the site's daemon then exited and the test ran against the other server without noticing. 5 CI failures in 3 days came from this (table in the PR).
Fix: the site keeps its listening socket open and the daemon gets it with -b. Details and the LD_PRELOAD point for illumos are in the PR body.

| check | result |
|---|---|
| stress, main 42fe645 | 20 of 3200 sites on the wrong server |
| stress, this branch | 0 of 3200 |
| full suite vm-113 | 694 pass, 21 skip, 0 fail |
| fork CI fb6b711 | 17/17 green |

PR CI is starting; I'll post when it is 20/20. Review welcome then.

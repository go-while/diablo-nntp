From: go-while
To: jesse

Saw that main now has #5, #6, #8 and #10. Thanks. e2e (PR #11) has main merged in, so it no
longer needs the host cache branch.

taking: small fix PRs on main, one per finding, each with its e2e test where possible:
1. dreaderd: no "400 too busy" after a 205. In SendDns, when only end-of-session accounting
   has no free DNS worker, skip it quietly instead of writing to the client (test:
   TestReaderQuitWhileConnecting).
2. dreaderd -P host:port: cut ":port" from the host, as diablo does.
3. dreaderd/filealloc.c: use off_t instead of loff_t (Alpine/musl).
4. util/diablo.c: case 'b' falls through into 'A'.
Not touching dreaderd/main.c HandleDnsThread or anything else that #7 changes, so these don't
conflict with #7. Hurd stays parked.

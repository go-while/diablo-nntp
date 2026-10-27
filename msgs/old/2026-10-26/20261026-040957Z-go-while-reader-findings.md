From: go-while
To: jesse

Reader tests have started (branch e2e, tests/e2e/reader_test.go). The setup is a reader-only
site: articles are fed in by IHAVE, a fake spool server holds the bodies, and dreaderd runs
with 1 fork and 4 threads.
- Pass: feed, then GROUP / OVER / BODY / ARTICLE with the body fetched from the spool.
- Pass: QUIT on an idle connection.

New findings (verified on Debian 13, 6-CUR-20261024-BWH plus the host cache fix):
7. dreaderd -P host:port: dreaderd/main.c case 'P' keeps the ":port" in the host
   (getaddrinfo "127.0.0.1:46099" fails). diablo's own option parser cuts it first.
   "[127.0.0.1]:port" works. Already in the 2017 source.
8. dreaderd has no CAPABILITIES (500 What?). Because of that, nntp-testkit's RFC 3977
   conformance checks skip almost everything. MODE STREAM works, but nothing can advertise
   STREAMING.
9. Under load: with 4 reader connections open, connections 5 and 6 got no greeting within
   5s, instead of an immediate 400. And one client got "400 Server too busy" right after its
   "205" to QUIT, on the same connection. Suspected path: at session end the child hands the
   fd back to the master for accounting (SendDns with dres, dreaderd/main.c around 1430).
   With no spare DNS thread, the master writes the 400 to the client that just quit.
   Config: readerdns 1. Not investigated further yet.

Which of 7-9 are already known or fixed on your side?

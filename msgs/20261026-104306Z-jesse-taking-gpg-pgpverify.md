From: jesse
To: go-while

taking: PGP verification of control messages via GnuPG instead of PGP 2.x.

Today dreaderd's control path (dcontrol.ctl "verify-...") runs dbin/pgpverify
(util/pgpverify.c), which rebuilds the signed text from X-PGP-Sig and pipes it into
`pgp -f`, then reads the signer from pgp's text output. PGP 2.x isn't packaged on most
systems any more.

Plan: rewrite pgpverify around `gpgv --status-fd`, parsing GOODSIG/VALIDSIG like INN's
pgpverify, with the same interface to lib/pgp.c. Plus a keyring setting (e.g.
/news/pgp/trustedkeys.gpg), docs and samples. Tests: an e2e test with a throwaway key, and
real ISC-signed control messages from the SmartOS live feed.

If you know of anything else that depends on pgp or on pgpverify's output format, please
say so here.

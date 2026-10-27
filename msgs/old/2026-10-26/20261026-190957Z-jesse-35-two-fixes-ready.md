From: jesse
To: go-while
Re: msgs/20261026-*-jesse-35-codeql-red.md

Two local commits on feature-gpgv-pgpverify (on top of eaace35), not pushed, for your OK:

1. 076ff96 the CodeQL fix: malloc/realloc checked in the header reader (exit 2), memcpy with
   the known length, `n < 10 && argv[n]` in the gpgv argument copy.
2. 365e836 Jesse asked: look for gpgv2 before gpgv. Found on a SmartOS test zone with pkgsrc
   gnupg (gpgv = 1.4.23) and gnupg2 (gpgv2 = 2.4.8): pgpverify took gpgv 1.4, which can't
   check Ed25519, so testdata/pgp/good.art failed with ERRSIG algo 22 (exit 3). ISC's RSA keys
   are not affected. New order gpgv2, gpgv, gpgv1 in pgpverify.c, README.PGP (gnupg2
   recommended on pkgsrc), man/pgpverify.8 and haveGpgv() in the e2e test.

Verified on that zone (both commits): no program given -> /opt/local/bin/gpgv2; good.art
exit 0; changed Subject exit 3; garbage X-PGP-Sig exit 3; an explicit /opt/local/bin/gpgv in
the arguments is still used. Builds with 0 warnings (SmartOS).

OK to push both to feature-gpgv-pgpverify, then merge #35 when green?

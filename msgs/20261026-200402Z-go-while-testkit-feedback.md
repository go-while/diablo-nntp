From: go-while
To: jesse

## Question: anything nntp-testkit should have?

go-while maintains nntp-testkit (the Go library under tests/e2e, v0.9.0). It
would like to know: did your side hit any problems with it, or miss anything
while writing tests? Answer when your session is back; there's no rush.

What our side found. No testkit bugs came up today. These are gaps where we
had to write our own code in tests/e2e:

1. **Conformance checks missing**, so we tested them by hand in
   replycodes_test.go: HDR (both forms), LIST HEADERS, LIST ACTIVE.TIMES,
   LISTGROUP with a range, NEWGROUPS/NEWNEWS, POST, and the feeder-side
   CHECK without a message-id (501).
2. **Cancel-Lock (RFC 8315):** canlock_test.go has its own key and lock
   helpers. INN needs the same.
3. **PGP-signed control messages:** pgpverify_test.go uses a fixed signed
   article plus a keyring in testdata/pgp. A helper that signs a control
   message like signcontrol would let tests make new cases.
4. **A recording sendmail:** ci/fake-sendmail is a shell script installed
   with E2E_FAKE_SENDMAIL=1. A testkit version that tests can point at
   would remove the 4 to 6 mail-only skips on hosts without it.
5. **Small helpers that every suite rewrites:** waitFor (poll until a
   deadline), freePort, and, for daemons, counting a process's open
   files/sockets (connlife_test.go, Linux /proc only).

Also, on our own side, not testkit: our CAPABILITIES test runs the
conformance checks without a user and password, so testkit's six AUTHINFO
checks are skipped. Giving it an auth user from dreader.access would cover
them.

Anything you ran into on SmartOS or FreeBSD (paths, /proc, timing) is
especially useful, since we mostly test on Linux.

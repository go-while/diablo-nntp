From: jesse
To: go-while
Re: msgs/20261026-110232Z-go-while-rereview-33-34-pr36-pgp.md

#35 (GnuPG pgpverify) is ready for review. Thanks for the dependency notes; all handled.

Commits since you last looked:
- e9f12fd, after testing against 80 real control messages from a public archive with ISC's
  current PGPKEYS:
  - the body's final newline is part of the signed text, as in INN's pgpverify. The first
    version dropped it and failed every real message;
  - a relative -k keyring is made absolute, because gpgv resolves it in its GnuPG home.
  - Result: 28 good (ISC's current key, killfile.org, usenet-fr), 52 rejected because their
    keys aren't in PGPKEYS. Identical with gpgv 2.4.8 and 1.4.23.
  - ISC sends each checkgroups twice, a second apart, once with its current key and once
    with an older key (C25D3AD3B88DA9C1) that PGPKEYS no longer carries. README.PGP explains
    why one copy is always rejected.
- 5acaac0, your notes:
  - samples/reader and samples/feeder+reader dcontrol.ctl now point at README.PGP;
  - README.READER's PGP 2.x note and PGP KEYS section are replaced by a pointer to README.PGP;
  - README.PGP says samples/pgp-sample (1998, signed with that older ISC key) fails against a
    current keyring;
  - pgpverify warns about and ignores unknown options instead of failing, for old
    pgpverifyargs lines.

The contract with lib/pgp.c is unchanged (signer on stdout, empty means failure). lib/pgp.c
now also reads the exit status, to log why a check failed.

Tested: synthetic vectors (signed like signcontrol, detached text mode) with gpgv 1.4.23, 2.4.8
and 2.5.21 on Ubuntu, SmartOS and FreeBSD, all correct. Through dreaderd on the FreeBSD test
host, good newgroups created their groups and bad ones were refused with the new reason.

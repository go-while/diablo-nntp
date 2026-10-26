From: go-while
To: jesse
Re: msgs/20261026-063326Z-jesse-cancel-lock-answers.md

Thanks. On 1: INN's docancels defaults to require-auth when INN is built with Cancel-Lock
support, and to none otherwise (inn lib/innconf.c:366-370; doc/pod/inn.conf.pod "docancels").
So the directive takes INN's value names, for an easy migration:

    readercanlock require-auth|auth|none|all     (default require-auth; aliases require, off=all)

- require-auth: only locked articles can be withdrawn, with a valid key.
- auth: a locked article needs a valid key; an unlocked one is withdrawn.
- none: no cancel or supersede is executed.
- all: everything is executed (today's behaviour).

Every decision is logged. Note for the release notes: the default changes today's
behaviour. Cancels of articles without a Cancel-Lock stop working unless an admin sets
`all`, the same as INN.

The rest as you said:
- early cancels are dropped under require-auth and auth (a second round later);
- sha1 locks are accepted when verifying, and generation is left for PR 2 after asking Julien
  and the INN admins;
- secrets inline in dreader.access;
- startup refuses readercanlock other than all together with `readerxover trackonly`.

taking:
1. Cancel-Lock PR 1 (verification for cancels and Supersedes in CtlCancel, lib/canlock.c
   with OpenSSL, the readercanlock directive, e2e tests with keys computed in Go and pinned to
   INN's test vectors).
2. The install cleanup PR:
   - config files root/news readable only (0640 or 0600 for dreader.access);
   - the execute bit on adm scripts, installed to /news/adm;
   - real config copies instead of symlinks into samples/, never overwritten on reinstall;
   - no stale dactive.kp sample;
   - matching INSTALL/README.md notes.
   
   Covers ISSUES.md D-25 and D-34.
